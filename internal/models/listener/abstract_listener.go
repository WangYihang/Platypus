package listener

import (
	"fmt"
	"net"

	"github.com/WangYihang/Platypus/internal/databases"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Listener struct {
	gorm.Model
	ID        string `json:"ID" gorm:"primaryKey"`
	Host      string `json:"host"`
	Port      uint16 `json:"port"`
	Enable    bool   `json:"enable" gorm:"default:false"`
	Protocol  string `json:"protocol"`
	NumAgents int    `json:"num_agents"`
	Comments  string `json:"comments"`
}

type Switch interface {
	Enable() (err error)
	Disable() (err error)
}

type ListenerInterface interface {
	Switch
	Handle(conn net.Conn)
}

var RunningListeners map[string]ListenerInterface

func Init() {
	if RunningListeners == nil {
		RunningListeners = make(map[string]ListenerInterface)
	}
}

func (l *Listener) BeforeCreate(tx *gorm.DB) (err error) {
	l.ID = uuid.New().String()
	return nil
}

func GetAllListeners() []Listener {
	var listeners []Listener
	databases.DB.Model(listeners).Find(&listeners)
	return listeners
}

func CreateListener(host string, port uint16, protocol string, enable bool) (*Listener, error) {
	var listener = Listener{
		Host:     host,
		Port:     port,
		Protocol: protocol,
	}

	if databases.DB.Model(listener).
		Where("host = ?", listener.Host).
		Where("port = ?", listener.Port).
		Where("protocol = ?", listener.Protocol).
		First(&listener).Error != nil {
		databases.DB.Create(&listener)
	}

	ResumeListener(listener)
	return &listener, nil
}

func SetListenerStatus(id string, new_status bool) (err error) {
	// Find Listener
	listener, err := GetListenerByID(id)
	if err != nil {
		return err
	}

	// Change Status
	if new_status {
		if val, ok := RunningListeners[listener.ID]; ok {
			err = val.Enable()
		}
	} else {
		if val, ok := RunningListeners[listener.ID]; ok {
			err = val.Disable()
		}
	}

	// Set New Status
	if err == nil {
		databases.DB.Model(&listener).Update("enable", new_status)
	}
	return err
}

func EnableListener(id string) (err error) {
	return SetListenerStatus(id, true)
}

func DisableListener(id string) (err error) {
	return SetListenerStatus(id, false)
}

func ResumeListener(listener Listener) {
	switch listener.Protocol {
	case "plain_tcp":
		RunningListeners[listener.ID] = &PlainTCPListener{Listener: listener, stopChan: make(chan bool), conn: make(chan net.Conn, 65535)}
	case "termite_tcp":
		RunningListeners[listener.ID] = &TermiteTCPListener{Listener: listener, stopChan: make(chan bool), connChan: make(chan net.Conn, 65535)}
	}
	err := SetListenerStatus(listener.ID, listener.Enable)
	if err != nil {
		fmt.Println(err)
	}
}

func ResumeAllListeners() {
	for _, listener := range GetAllListeners() {
		ResumeListener(listener)
	}
}

func GetListenerByID(id string) (*Listener, error) {
	var listener = Listener{}
	result := databases.DB.Model(listener).Where("ID = ?", id).First(&listener)
	if result.RowsAffected == 0 {
		return nil, fmt.Errorf("no such listener")
	} else {
		return &listener, nil
	}
}

func CheckListenerExists(host string, port uint16, protocol string) bool {
	var listener = Listener{}
	result := databases.DB.Model(listener).
		Where("host = ?", host).
		Where("port = ?", port).
		Where("protocol = ?", protocol).
		First(&listener)
	return result.Error == nil
}
