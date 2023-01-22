package controllers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	agent_model "github.com/WangYihang/Platypus/internal/models/agent"
	record_model "github.com/WangYihang/Platypus/internal/models/record"
	"github.com/WangYihang/Platypus/internal/utils/asciinema"
	http_util "github.com/WangYihang/Platypus/internal/utils/http"
	"github.com/WangYihang/Platypus/internal/utils/log"
	"github.com/WangYihang/Platypus/internal/utils/ui"
	"github.com/gin-gonic/gin"
	"gopkg.in/olahol/melody.v1"
)

func GetAllAgents(c *gin.Context) {
	c.IndentedJSON(http.StatusOK, gin.H{
		"status":  true,
		"message": agent_model.GetAllAgents(),
	})
}

func SpawnWebsocketTTY(c *gin.Context) {
	if !http_util.CheckIDExists(c) {
		c.AbortWithStatusJSON(http.StatusNotFound, gin.H{
			"status":  false,
			"message": "Invalid agent ID",
		})
		return
	}

	agentConn, err := agent_model.GetAgentConnByID(c.Param("id"))
	if err != nil {
		c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "Agent not online",
		})
		return
	}
	conn := agentConn.GetConn()

	ws := melody.New()

	ac := asciinema.AsciinemaCast{
		Version: 1,
		Command: "/bin/bash",
		Title:   "this is a title",
		Env:     make(map[string]string),
		Stdout:  []asciinema.AsciinemaStdout{},
	}
	start := time.Now()
	prev := time.Now()
	address := agentConn.GetConn().RemoteAddr().String()

	ws.HandleConnect(func(s *melody.Session) {
		log.Success("[%s] connected", address)

		// SET_WINDOW_TITLE '1'
		s.WriteBinary([]byte("1" + "/bin/bash (ubuntu)"))
		// SET_PREFERENCES '2'
		s.WriteBinary([]byte("2" + "{ }"))

		go func(s *melody.Session) {
			for !s.IsClosed() {
				msg := make([]byte, 1024)
				n, err := conn.Read(msg)
				if err != nil {
					log.Error("Read from socket failed: %s", err)
					return
				}

				curr := time.Now()
				ac.Record(curr.Sub(prev).Seconds(), []byte(msg[0:n]))
				prev = curr
				s.WriteBinary([]byte("0" + string(msg[0:n])))
			}
		}(s)
	})

	ws.HandleMessageBinary(func(s *melody.Session, msg []byte) {
		log.Success("[%s] sent message", address)

		opcode := msg[0]
		body := msg[1:]
		switch opcode {
		case '0': // INPUT '0'
			agentConn.GetConn().Write(body)
		case '1': // RESIZE_TERMINAL '1'
			fmt.Print("RESIZE_TERMINAL")
			var ws ui.WindowSize
			json.Unmarshal(body, &ws)
			if ws.Columns > ac.Wdith {
				ac.Wdith = ws.Columns
			}
			if ws.Rows > ac.Height {
				ac.Height = ws.Rows
			}
			agentConn.ResetWindow(ws.Columns, ws.Rows)
		case '2': // PAUSE '2'
			fmt.Print("PAUSE")
			// TODO: Pause, support for zmodem
		case '3': // RESUME '3'
			fmt.Print("RESUME")
			// TODO: Pause, support for zmodem
		case '{': // JSON_DATA '{'
			fmt.Print("JSON_DATA")
		default:
			fmt.Println("Invalid message: ", string(msg))
		}
	})

	ws.HandleDisconnect(func(s *melody.Session) {
		log.Success("[%s] disconnected", address)

		agent, err := agent_model.GetAgentByID(agentConn.GetID())
		fmt.Println(">>>", agent, err)
		if err != nil {
			fmt.Println(err.Error())
			return
		}
		record, err := record_model.CreateRecord(agent)
		fmt.Println(">>>", record, err)
		if err != nil {
			fmt.Println(err.Error())
			return
		}
		ac.Duration = time.Since(start).Seconds()
		ac.Save(fmt.Sprintf("records/%s.cast", record.ID))
	})

	ws.HandleRequest(c.Writer, c.Request)
}

func GetAgent(c *gin.Context) {
	c.String(http.StatusInternalServerError, "To be implemented")
}

func CollectAgentInfo(c *gin.Context) {
	c.String(http.StatusInternalServerError, "To be implemented")
}

func GetAllProxies(c *gin.Context) {
	c.String(http.StatusInternalServerError, "To be implemented")
}

func CreateProxy(c *gin.Context) {
	c.String(http.StatusInternalServerError, "To be implemented")
}

func DeleteProxy(c *gin.Context) {
	c.String(http.StatusInternalServerError, "To be implemented")
}

func StartProxy(c *gin.Context) {
	c.String(http.StatusInternalServerError, "To be implemented")
}

func StopProxy(c *gin.Context) {
	c.String(http.StatusInternalServerError, "To be implemented")
}

func LibReadDir(c *gin.Context) {
	c.String(http.StatusInternalServerError, "To be implemented")
}

func LibStat(c *gin.Context) {
	c.String(http.StatusInternalServerError, "To be implemented")
}

func LibReadFile(c *gin.Context) {
	c.String(http.StatusInternalServerError, "To be implemented")
}

func LibWriteFile(c *gin.Context) {
	c.String(http.StatusInternalServerError, "To be implemented")
}

func LibFopen(c *gin.Context) {
	c.String(http.StatusInternalServerError, "To be implemented")
}

func LibFseek(c *gin.Context) {
	c.String(http.StatusInternalServerError, "To be implemented")
}

func LibFread(c *gin.Context) {
	c.String(http.StatusInternalServerError, "To be implemented")
}

func LibFwrite(c *gin.Context) {
	c.String(http.StatusInternalServerError, "To be implemented")
}

func LibFclose(c *gin.Context) {
	c.String(http.StatusInternalServerError, "To be implemented")
}

func InstallCrontab(c *gin.Context) {
	c.String(http.StatusInternalServerError, "To be implemented")
}

func InstallSshKey(c *gin.Context) {
	c.String(http.StatusInternalServerError, "To be implemented")
}

func UpgradeToTermite(c *gin.Context) {
	c.String(http.StatusInternalServerError, "To be implemented")
}

func DeleteClient(c *gin.Context) {
	c.String(http.StatusInternalServerError, "To be implemented")
}
