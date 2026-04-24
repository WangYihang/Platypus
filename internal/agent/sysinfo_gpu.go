package agent

import (
	"bytes"
	"context"
	"encoding/csv"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/jaypipes/ghw"

	v2pb "github.com/WangYihang/Platypus/pkg/proto/v2"
)

// collectGPUs enumerates graphics adapters and (when possible)
// enriches each with live NVIDIA runtime stats. Static fields come
// from ghw — pure /sys reads on Linux and WMI on Windows; on macOS
// ghw returns nothing so we add a single synthetic Apple entry.
//
// This file contains the *one* shell-out in the agent's sysinfo
// collection path (`nvidia-smi`). Existing code avoids shelling out
// for best-effort probes (see `detectDefaultGateway`), but GPU
// utilization and VRAM-used are not exposed via /sys/class/drm on
// NVIDIA, and a CGO-free NVML binding isn't feasible — so we make a
// deliberate exception, guarded by exec.LookPath and a 1-second
// timeout so a wedged driver can't stall the snapshot.
func collectGPUs(ctx context.Context) []*v2pb.GPUInfo {
	defer func() {
		// ghw occasionally panics on hosts with unusual sysfs
		// layouts. Swallow and continue — a missing GPU list never
		// fails the whole snapshot.
		_ = recover()
	}()

	gpus := readGHWGPUs()
	if runtime.GOOS == "darwin" && len(gpus) == 0 {
		gpus = darwinSyntheticGPUs()
	}
	enrichNvidiaSmi(ctx, gpus)
	return gpus
}

// readGHWGPUs wraps ghw.GPU() and maps each graphics card into a
// GPUInfo proto. ghw returns vendor + product via the PCI DB lookup
// and driver via the sysfs driver symlink — none of which require
// shelling out.
func readGHWGPUs() []*v2pb.GPUInfo {
	info, err := ghw.GPU(ghw.WithDisableWarnings())
	if err != nil || info == nil {
		return nil
	}
	out := make([]*v2pb.GPUInfo, 0, len(info.GraphicsCards))
	for i, card := range info.GraphicsCards {
		g := &v2pb.GPUInfo{Index: uint32(i)}
		if card.DeviceInfo != nil {
			if card.DeviceInfo.Vendor != nil {
				g.Vendor = card.DeviceInfo.Vendor.Name
			}
			if card.DeviceInfo.Product != nil {
				g.Model = card.DeviceInfo.Product.Name
			}
			if card.DeviceInfo.Driver != "" {
				g.Driver = card.DeviceInfo.Driver
			}
		}
		if card.Address != "" {
			g.BusId = card.Address
		}
		out = append(out, g)
	}
	return out
}

// darwinSyntheticGPUs builds a single, static entry for the
// integrated GPU on Apple Silicon. Vendor/model are inferred from
// the system architecture; VRAM/utilization would require private
// APIs we don't ship.
func darwinSyntheticGPUs() []*v2pb.GPUInfo {
	return []*v2pb.GPUInfo{{
		Vendor: "Apple",
		Model:  "Apple Silicon GPU",
		Driver: "AGX",
	}}
}

// enrichNvidiaSmi runs `nvidia-smi --query-gpu=...` when the binary
// is on PATH, then merges the per-card utilization / memory / driver
// fields into the already-populated slice. Matching happens by PCI
// bus id first, then by index order. Every failure path is a no-op
// — nvidia-smi is purely advisory.
func enrichNvidiaSmi(ctx context.Context, gpus []*v2pb.GPUInfo) {
	if len(gpus) == 0 {
		return
	}
	bin, err := exec.LookPath("nvidia-smi")
	if err != nil {
		return
	}

	runCtx, cancel := context.WithTimeout(ctx, 1*time.Second)
	defer cancel()

	// Keep the field set tight — the more columns we ask for, the
	// more parse failures we invite across driver versions.
	args := []string{
		"--query-gpu=index,pci.bus_id,name,driver_version,memory.total,memory.used,utilization.gpu,uuid",
		"--format=csv,noheader,nounits",
	}
	cmd := exec.CommandContext(runCtx, bin, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	if err := cmd.Run(); err != nil {
		return
	}

	reader := csv.NewReader(&buf)
	reader.TrimLeadingSpace = true
	rows, err := reader.ReadAll()
	if err != nil {
		return
	}

	byBus := map[string]*v2pb.GPUInfo{}
	for _, g := range gpus {
		if g.BusId != "" {
			byBus[strings.ToLower(normalizeNvidiaBusID(g.BusId))] = g
		}
	}

	for rowIdx, row := range rows {
		if len(row) < 8 {
			continue
		}
		idx, _ := strconv.ParseUint(strings.TrimSpace(row[0]), 10, 32)
		bus := strings.ToLower(normalizeNvidiaBusID(row[1]))
		target := byBus[bus]
		if target == nil && int(idx) < len(gpus) {
			target = gpus[int(idx)]
		}
		if target == nil && rowIdx < len(gpus) {
			target = gpus[rowIdx]
		}
		if target == nil {
			continue
		}
		if target.Vendor == "" {
			target.Vendor = "NVIDIA"
		}
		if target.Model == "" {
			target.Model = strings.TrimSpace(row[2])
		}
		target.DriverVersion = strings.TrimSpace(row[3])
		if memTotalMiB, err := strconv.ParseUint(strings.TrimSpace(row[4]), 10, 64); err == nil {
			target.VramTotalBytes = memTotalMiB * 1024 * 1024
		}
		if memUsedMiB, err := strconv.ParseUint(strings.TrimSpace(row[5]), 10, 64); err == nil {
			target.VramUsedBytes = memUsedMiB * 1024 * 1024
		}
		if util, err := strconv.ParseFloat(strings.TrimSpace(row[6]), 64); err == nil {
			target.UtilizationPct = util
		}
		if uuid := strings.TrimSpace(row[7]); uuid != "" {
			target.Uuid = uuid
		}
	}
}

// normalizeNvidiaBusID reduces the various bus-id formats ghw and
// nvidia-smi report to a common lower-case form. ghw reports the PCI
// address as "0000:65:00.0"; nvidia-smi reports it as
// "00000000:65:00.0" (eight-digit domain). Strip any leading zeros
// in the domain so the two match.
func normalizeNvidiaBusID(id string) string {
	s := strings.TrimSpace(id)
	if s == "" {
		return s
	}
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return strings.ToLower(s)
	}
	domain := strings.TrimLeft(parts[0], "0")
	if domain == "" {
		domain = "0"
	}
	return strings.ToLower(domain + ":" + parts[1])
}
