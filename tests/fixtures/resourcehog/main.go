package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"
)

func main() {
	mode := flag.String("mode", "idle", "idle, cpu, stubborn-cpu, memory, processes, threads, or probe")
	counter := flag.String("counter", "", "invocation counter file")
	breachCount := flag.Int("breach-count", 0, "number of invocations that use the requested resource")
	pid := flag.Int("pid", 0, "PID for probe mode")
	trigger := flag.String("trigger", "", "file that activates a switch mode")
	flag.Parse()
	if *mode == "probe" {
		if *pid <= 1 || syscall.Kill(*pid, 0) != nil {
			os.Exit(1)
		}
		return
	}
	if !shouldBreach(*counter, *breachCount) {
		block()
	}
	switch *mode {
	case "cpu":
		burnCPU()
	case "stubborn-cpu":
		signal.Ignore(syscall.SIGTERM)
		burnCPU()
	case "memory":
		memory := make([]byte, 128<<20)
		for index := 0; index < len(memory); index += 4096 {
			memory[index] = 1
		}
		for {
			runtime.KeepAlive(memory)
			time.Sleep(time.Second)
		}
	case "processes":
		spawnChildren(6)
	case "switch-processes":
		for {
			if _, err := os.Lstat(*trigger); err == nil {
				spawnChildren(40)
			}
			time.Sleep(100 * time.Millisecond)
		}
	case "threads":
		for range 32 {
			go func() {
				runtime.LockOSThread()
				block()
			}()
		}
		block()
	default:
		block()
	}
}

func spawnChildren(count int) {
	children := make([]*exec.Cmd, 0, count)
	for range count {
		child := exec.Command("sleep", "300")
		if err := child.Start(); err != nil {
			panic(err)
		}
		children = append(children, child)
	}
	for _, child := range children {
		_ = child.Wait()
	}
}

func shouldBreach(path string, maximum int) bool {
	if path == "" || maximum == 0 {
		return true
	}
	value := 0
	if content, err := os.ReadFile(path); err == nil {
		value, _ = strconv.Atoi(string(content))
	}
	value++
	if err := os.WriteFile(path, []byte(fmt.Sprintf("%d", value)), 0o600); err != nil {
		panic(err)
	}
	return value <= maximum
}

func block() {
	for {
		time.Sleep(time.Hour)
	}
}

func burnCPU() {
	value := uint64(1)
	for {
		for range 1_000_000 {
			value = value*6364136223846793005 + 1
		}
		runtime.KeepAlive(value)
	}
}
