package agent

import (
	"fmt"
	"net"
	"time"

	"github.com/WangYihang/Platypus/internal/databases"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Agent struct {
	gorm.Model
	ID          string    `json:"ID" gorm:"primaryKey"`
	OS          string    `json:"os"`
	Arch        string    `json:"arch"`
	IP          string    `json:"ip"`
	Port        uint16    `json:"port"`
	Username    string    `json:"username"`
	Comments    string    `json:"comments"`
	Version     string    `json:"version"`
	ConnectedAt time.Time `json:"connected_at"`
	LastSeen    time.Time `json:"last_seen"`
	Python2     string    `json:"python2"`
	Python3     string    `json:"python3"`
	Protocol    string    `json:"protocol"`
	UniqueID    string    `json:"unique_id"`
}

type AgentConn struct {
	Agent
	Conn net.Conn
}

type AgentInterface interface {
	Setup()
	GetConn() net.Conn
	GetID() string
	System(command string) string
	Download(remotePath string, localPath string)
	Upload(localPath string, remotePath string)
	Handle()
	ResetWindow(width int, height int)
}

var OnlineAgents map[string]AgentInterface

func Init() {
	if OnlineAgents == nil {
		OnlineAgents = make(map[string]AgentInterface)
	}
}

func (a *Agent) BeforeCreate(tx *gorm.DB) (err error) {
	a.ID = uuid.New().String()
	return nil
}

func GetAllAgents() *[]Agent {
	var agents []Agent
	databases.DB.Find(&agents)
	return &agents
}

func GetAgentByID(id string) (*Agent, error) {
	var agent = Agent{}
	result := databases.DB.Model(agent).Where("ID = ?", id).First(&agent)
	if result.RowsAffected == 0 {
		return nil, fmt.Errorf("no such agent")
	} else {
		return &agent, nil
	}
}

func GetAgentConnByID(id string) (AgentInterface, error) {
	if agent, ok := OnlineAgents[id]; ok {
		return agent, nil
	} else {
		return nil, fmt.Errorf("no such agent")
	}
}

func CreateAgent(
	os string, arch string,
	ip string, port uint16,
	username string, comments string,
	version string, protocol string,
	python2 string, python3 string,
	conn net.Conn,
) {
	var agent = Agent{
		OS:          os,
		Arch:        arch,
		IP:          ip,
		Port:        port,
		Username:    username,
		Comments:    comments,
		Version:     version,
		ConnectedAt: time.Now(),
		LastSeen:    time.Now(),
		Python2:     python2,
		Python3:     python3,
		Protocol:    protocol,
	}
	databases.DB.Model(&agent).Create(&agent)
	switch protocol {
	case "plain_tcp":
		OnlineAgents[agent.ID] = &PlainTCPAgent{AgentConn{Agent: agent, Conn: conn}}
		// case "plain_udp":
		// 	OnlineAgents[agent.ID] = &PlainUDPAgent{AgentConn{Agent: agent, Conn: conn}}
		// case "termite_tcp":
		// 	OnlineAgents[agent.ID] = &TermiteTCPAgent{AgentConn{Agent: agent, Conn: conn}}
		// case "termite_udp":
		// 	OnlineAgents[agent.ID] = &TermiteUDPAgent{AgentConn{Agent: agent, Conn: conn}}
		// case "termite_dns":
		// 	OnlineAgents[agent.ID] = &TermiteDNSAgent{AgentConn{Agent: agent, Conn: conn}}
		// case "termite_icmp":
		// 	OnlineAgents[agent.ID] = &TermiteICMPAgent{AgentConn{Agent: agent, Conn: conn}}
	}
	fmt.Println(agent.ID)
	go OnlineAgents[agent.ID].Handle()
}

func System(id string, command string) (string, error) {
	// Find Agent
	agent, err := GetAgentByID(id)
	if err != nil {
		return "", err
	}

	// Execute command on the target agent
	if val, ok := OnlineAgents[agent.ID]; ok {
		return val.System(id), nil
	}

	return "", fmt.Errorf("Agent not online")
}
