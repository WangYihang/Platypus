package asciinema

import (
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/WangYihang/Platypus/internal/utils/log"
)

type AsciinemaStdout struct {
	Delay float64 `json:"delay"`
	Data  []byte  `json:"data"`
}

type AsciinemaCast struct {
	Version  int               `json:"version"`
	Wdith    int               `json:"wdith"`
	Height   int               `json:"height"`
	Duration float64           `json:"duration"`
	Command  string            `json:"command"`
	Title    string            `json:"title"`
	Env      map[string]string `json:"env"`
	Stdout   []AsciinemaStdout `json:"stdout"`
}

func (ac *AsciinemaCast) Save(filepath string) {
	data, err := json.Marshal(ac)
	if err != nil {
		fmt.Println(err.Error())
	}
	err = ioutil.WriteFile(filepath, data, 0644)
	if err != nil {
		fmt.Println(err.Error())
	}
	log.Info("Record saved to %s", filepath)
}

func (ac *AsciinemaCast) Record(delay float64, data []byte) {
	stdout := AsciinemaStdout{
		Delay: delay,
		Data:  data,
	}
	ac.Stdout = append(ac.Stdout, stdout)
}

func (as *AsciinemaStdout) MarshalJSON() ([]byte, error) {
	data, _ := json.Marshal(string(as.Data))
	return []byte(fmt.Sprintf("[%f, %s]", as.Delay, data)), nil
}
