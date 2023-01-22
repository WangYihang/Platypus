package record

import (
	"fmt"
	"io/ioutil"

	"github.com/WangYihang/Platypus/internal/databases"
	agent_model "github.com/WangYihang/Platypus/internal/models/agent"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type Record struct {
	gorm.Model
	ID         string            `json:"ID" gorm:"primaryKey"`
	AgentRefer string            `json:"-"`
	Agent      agent_model.Agent `json:"-" gorm:"foreignKey:AgentRefer"`
}

func (r *Record) BeforeCreate(tx *gorm.DB) (err error) {
	r.ID = uuid.New().String()
	return nil
}

func RawRecord(id string) ([]byte, error) {
	filepath := fmt.Sprintf("records/%s.cast", id)
	content, err := ioutil.ReadFile(filepath)
	if err != nil {
		return []byte(""), err
	}
	return content, nil
}

func CreateRecord(agent *agent_model.Agent) (*Record, error) {
	var record = Record{
		Agent: *agent,
	}

	if result := databases.DB.Create(&record); result.RowsAffected == 0 {
		return nil, fmt.Errorf("create record failed")
	}
	return &record, nil
}

func GetAllRecords() []Record {
	var records []Record
	databases.DB.Model(records).Find(&records)
	return records
}

func GetRecordByID(id string) (*Record, error) {
	var record = Record{}
	result := databases.DB.Model(record).Where("ID = ?", id).First(&record)
	if result.RowsAffected == 0 {
		return nil, fmt.Errorf("no such record")
	} else {
		return &record, nil
	}
}
