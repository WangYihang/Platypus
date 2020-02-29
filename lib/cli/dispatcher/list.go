package dispatcher

import (
	"fmt"
	"os"

	"github.com/WangYihang/Platypus/lib/context"
	"github.com/WangYihang/Platypus/lib/util/log"
	"github.com/dustin/go-humanize"
	"github.com/jedib0t/go-pretty/table"
)

func (dispatcher Dispatcher) List(args []string) {
	if len(context.Ctx.Servers) == 0 {
		log.Warn(fmt.Sprintf("No listening servers"))
		return
	}
	log.Info(fmt.Sprintf("Listing %d listening servers", len(context.Ctx.Servers)))

	for shash, server := range context.Ctx.Servers {
		if len(server.Clients) > 0 {
			t := table.NewWriter()
			t.SetOutputMirror(os.Stdout)
			t.SetTitle(fmt.Sprintf(
				"[%s] Listening on %s:%d, %d Clients",
				shash,
				(*server).Host,
				(*server).Port,
				len((*server).Clients),
			))
			t.AppendHeader(table.Row{"ID", "Hash", "Network", "OS", "Time"})
			i := 0
			for chash, client := range server.Clients {
				i++
				t.AppendRow([]interface{}{
					i,
					chash,
					client.Conn.RemoteAddr().String(),
					client.OS,
					humanize.Time(client.TimeStamp),
				})
			}
			t.Render()
		} else {
			log.Warn(fmt.Sprintf(
				"[%s] listening on %s:%d, 0 clients",
				shash,
				(*server).Host,
				(*server).Port,
			))
		}
	}
}

func (dispatcher Dispatcher) ListHelp(args []string) {
	fmt.Println("Usage of List")
	fmt.Println("\tList")
}

func (dispatcher Dispatcher) ListDesc(args []string) {
	fmt.Println("List")
	fmt.Println("\tTry list all listening servers and connected clients")
}
