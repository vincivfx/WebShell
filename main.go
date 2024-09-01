package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"
)

type Execution struct {
	Command    string
	Args       []string
	Output     string
	Code       int
	Terminated bool
	Killed     bool
	Started    time.Time
	Ended      time.Time
	process    *exec.Cmd
}

var InternalDB map[string]*Execution

type AppConfiguration struct {
	Timeout   int      `json:"timeout"`
	Programs  []string `json:"programs"`
	CacheTime int	   `json:"cacheTime"`
}

type CommandRequestBody struct {
	Command string   `json:"command"`
	Args    []string `json:"args"`
}

type CommandStatusBody struct {
	Key string `json:"key"`
}

type CommandKillBody struct {
	Key string `json:"key"`
}

func getBody(v io.ReadCloser) []byte {
	body, err := io.ReadAll(v)

	if err != nil {
		return nil
	}

	return body
}

func handleCommand(key string) {
	fmt.Printf("Handing command %s as %s\n", InternalDB[key].Command, key)

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(config.Timeout)*time.Second)
	defer cancel()

	command := exec.CommandContext(ctx, InternalDB[key].Command, InternalDB[key].Args...)

	InternalDB[key].process = command

	output, err := command.CombinedOutput()
	if err != nil {
		InternalDB[key].Killed = true
		fmt.Printf("Command %s failed: %s\n", InternalDB[key].Command, err)
	}

	InternalDB[key].Output = string(output)
	InternalDB[key].Code = command.ProcessState.ExitCode()
	InternalDB[key].Ended = time.Now()
	InternalDB[key].Terminated = true
	

}

func CommandRequest(w http.ResponseWriter, r *http.Request) {

	bodyRequest := CommandRequestBody{}
	jsonError := json.Unmarshal(getBody(r.Body), &bodyRequest)

	if jsonError != nil {
		w.WriteHeader(500)
		w.Write([]byte("internal server error"))
		return
	}

	found := false

	for _, value := range config.Programs {
		if bodyRequest.Command == value {
			found = true
			break
		}
	}

	if !found {
		w.WriteHeader(404)
		w.Write([]byte("command not found"))
		return
	}

	commandKey := uuid.New().String()

	exec := Execution{
		Command:    bodyRequest.Command,
		Args:       bodyRequest.Args,
		Started:    time.Now(),
		Killed:     false,
		Terminated: false,
	}

	InternalDB[commandKey] = &exec

	go handleCommand(commandKey)

	w.WriteHeader(200)
	w.Write([]byte(commandKey))

}

func CommandStatus(w http.ResponseWriter, r *http.Request) {

	bodyRequest := CommandStatusBody{}
	jsonError := json.Unmarshal(getBody(r.Body), &bodyRequest)

	if jsonError != nil {
		w.WriteHeader(500)
		w.Write([]byte("internal server error"))
		return
	}

	command, ok := InternalDB[bodyRequest.Key]

	if !ok {
		w.WriteHeader(404)
		w.Write([]byte("command not found"))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(200)
	info, _ := json.Marshal(command)

	w.Write(info)

}
func CommandKill(w http.ResponseWriter, r *http.Request) {

	bodyRequest := CommandKillBody{}
	jsonError := json.Unmarshal(getBody(r.Body), &bodyRequest)

	if jsonError != nil {
		w.WriteHeader(500)
		w.Write([]byte("internal server error"))
		return
	}

	command, ok := InternalDB[bodyRequest.Key]

	if !ok {
		w.WriteHeader(404)
		w.Write([]byte("command not found"))
		return
	}

	if command.Terminated || command.Killed || command.process == nil {
		w.WriteHeader(200)
		w.Write([]byte("already killed"))
		return
	}

	killError := command.process.Process.Kill()

	if killError != nil {
		w.WriteHeader(500)
		w.Write([]byte("internal server error"))
		return
	}

	command.Terminated = true
	command.Killed = true
	command.Ended = time.Now()

	w.WriteHeader(200)
	w.Write([]byte("killed!"))

}

var config AppConfiguration

func memoryCleaner() {

	for ;; {

		// Remove entry in InternalDB after CacheTime seconds (specified in config.json)
		for key, value := range InternalDB {
			if value.Terminated && value.Ended.Add(time.Duration(config.CacheTime) * time.Second).Compare(time.Now()) < 0 {
				delete(InternalDB, key)
			}
		}

		time.Sleep(10 * time.Second)
	}

}

func main() {

	InternalDB = make(map[string]*Execution)

	go memoryCleaner() // Keep clean InternalDB

	configFileContents, configFileError := os.ReadFile("./config/config.json")

	if configFileError != nil {
		fmt.Println("Error opening config file: ", configFileError.Error())
		os.Exit(1)
	}

	jsonError := json.Unmarshal(configFileContents, &config)

	if jsonError != nil {
		fmt.Println("Error parsing config file: ", jsonError.Error())
		os.Exit(1)
	}

	http.HandleFunc("/request", CommandRequest)
	http.HandleFunc("/status", CommandStatus)
	http.HandleFunc("/kill", CommandKill)

	serverError := http.ListenAndServe("0.0.0.0:8080", nil)

	if serverError != nil {
		fmt.Println(serverError)
		os.Exit(1)
	}

	os.Exit(0)

}
