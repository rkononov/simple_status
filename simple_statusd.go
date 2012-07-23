package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"time"
)

var (
	port  = flag.String("p", ":8080", "http service address")
	token = flag.String("t", "", "http auth token")
	tls   bool
)

type Message struct {
	Host     string
	Load     string
	Rams     string
	Time     string
	Tasklist string
}

func init() {
	flag.BoolVar(&tls, "ssl", false, "TLS boolean flag")
	flag.Parse()
}
func Pipeline(cmds ...*exec.Cmd) (pipeLineOutput, collectedStandardError []byte, pipeLineError error) {
	// Require at least one command
	if len(cmds) < 1 {
		return nil, nil, nil
	}

	// Collect the output from the command(s)
	var output bytes.Buffer
	var stderr bytes.Buffer

	last := len(cmds) - 1
	for i, cmd := range cmds[:last] {
		var err error
		// Connect each command's stdin to the previous command's stdout
		if cmds[i+1].Stdin, err = cmd.StdoutPipe(); err != nil {
			return nil, nil, err
		}
		// Connect each command's stderr to a buffer
		cmd.Stderr = &stderr
	}

	// Connect the output and error for the last command
	cmds[last].Stdout, cmds[last].Stderr = &output, &stderr

	// Start each command
	for _, cmd := range cmds {
		if err := cmd.Start(); err != nil {
			return output.Bytes(), stderr.Bytes(), err
		}
	}

	// Wait for each command to complete
	for _, cmd := range cmds {
		if err := cmd.Wait(); err != nil {
			return output.Bytes(), stderr.Bytes(), err
		}
	}

	// Return the pipeline output and the collected standard error
	return output.Bytes(), stderr.Bytes(), nil
}
func host() string {
	host, err := os.Hostname()
	if err != nil {
		return fmt.Sprint(err)
	}
	return host
}

func tasklist() string {
	psax := exec.Command("ps", "ax")
	grep := exec.Command("grep", " 'su nobody'")
	// Run the pipeline
	tasklist, stderr, err := Pipeline(psax, grep)
	if err != nil {
		return fmt.Sprint(err)
	}
	if len(stderr) > 0 {
		log.Printf("stderr\n%s", stderr)
	}
	return fmt.Sprintf("%s", tasklist[:len(tasklist)-1])
}

func load() string {
	b, err := ioutil.ReadFile("/proc/loadavg")
	if err != nil {
		return fmt.Sprint(err)
	}
	return fmt.Sprintf("%s", b[:len(b)-1])
}

func ram() string {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return fmt.Sprint(err)
	}
	defer f.Close()

	bufReader := bufio.NewReader(f)
	b := make([]byte, 100)
	var free, total string
	for line, isPrefix, err := bufReader.ReadLine(); err != io.EOF; line, isPrefix, err = bufReader.ReadLine() {
		b = append(b, line...)

		if !isPrefix {
			switch {
			case bytes.Contains(b, []byte("MemFree")):
				s := bytes.Fields(b)
				free = string(s[1])
			case bytes.Contains(b, []byte("MemTotal")):
				s := bytes.Fields(b)
				total = string(s[1])
			}
			b = b[:0]
		}
	}
	return fmt.Sprintf("%s/%s", free, total)
}

func now() string {
	return time.Now().Format("2006 01/02 1504-05")
}

func message() []byte {
	m := Message{host(), load(), ram(), now(), tasklist()}
	b, err := json.Marshal(m)
	if err != nil {
		return nil
	}
	return b
}

func handler(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "%s", message())
}

func main() {
	url := "/"
	if *token != "" {
		url += *token
	}
	http.HandleFunc(url, handler)
	switch tls {
	case false:
		err := http.ListenAndServe(*port, nil)
		if err != nil {
			log.Fatal("ListenAndServe:", err)
		}
	case true:
		err := http.ListenAndServeTLS(*port, "cert.pem", "key.pem", nil)
		if err != nil {
			log.Fatal("ListenAndServeTLS:", err)
		}
	}
}
