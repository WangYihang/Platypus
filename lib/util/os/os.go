package os

type OperatingSystem int

const (
	Unknown OperatingSystem = iota
	Linux
	Windows
	WindowsPowerShell
	SunOS
	MacOS
	FreeBSD
)

func (os OperatingSystem) String() string {
	return [...]string{"Unknown", "🐧", "❖", "❖ [PowerShell]", "SunOS", "🍎", "FreeBSD"}[os]
}

func Parse(osstr string) OperatingSystem {
	table := map[string]OperatingSystem{
		"linux":   Linux,
		"windows": Windows,
		"darwin":  MacOS,
		"freebsd": FreeBSD,
	}
	if value, ok := table[osstr]; ok {
		return value
	} else {
		return Unknown
	}
}
