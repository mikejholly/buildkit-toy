package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/fatih/color"
	"github.com/moby/buildkit/client"
	"github.com/moby/buildkit/client/llb"
	"gopkg.in/yaml.v2"
)

type noobfile struct {
	With     string   `yaml:"with"`
	Commands []string `yaml:"commands"`
}

func main() {
	var file = flag.String("file", "", "File location of the commands to run")
	flag.Parse()

	f, err := os.Open(*file)
	if err != nil {
		log.Fatalf("failed to open command file: %v", err)
	}
	defer f.Close()

	nf := &noobfile{}

	err = yaml.NewDecoder(f).Decode(nf)
	if err != nil {
		log.Fatalf("failed to load file contents: %v", err)
	}

	if nf.With == "" {
		log.Fatalf("no image specified")
	}

	state := llb.Image(nf.With)
	local := llb.Local("local-pwd")

	if len(nf.Commands) == 0 {
		log.Fatalf("file does not contain a list of commands")
	}

	for _, command := range nf.Commands {
		parts := strings.SplitN(command, " ", 2)
		switch parts[0] {
		case "env":
			state = handleEnv(state, parts[1])
		case "cp":
			state = handleCp(local, state, parts[1])
		case "execute":
			state = handleExecute(state, parts[1])
		default:
			log.Fatalf("%q is not a valid command", parts[0])
		}
	}

	ctx := context.Background()

	llbDef, err := state.Marshal(ctx)
	if err != nil {
		log.Fatalf("failed to marshal llb: %v", err)
	}

	bkClient, err := client.New(ctx, "tcp://127.0.0.1:7000")
	if err != nil {
		log.Fatalf("failed to connect to buildkitd: %v", err)
	}
	defer bkClient.Close()

	statusChan := make(chan *client.SolveStatus)

	blue := color.New(color.FgBlue).SprintFunc()
	green := color.New(color.FgGreen).SprintFunc()

	go func() {
		for status := range statusChan {
			if status == nil {
				break
			}
			for _, l := range status.Logs {
				fmt.Printf("%s\n%v\n", blue("Output:"), string(l.Data))
			}
			for _, v := range status.Vertexes {
				fmt.Printf("%s %v\n", green("Doing:"), v.Name)
			}
		}
	}()

	pwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("failed to get working dir: %v", err)
	}

	_, err = bkClient.Solve(ctx, llbDef, client.SolveOpt{
		LocalDirs: map[string]string{
			"local-pwd": pwd,
		},
	}, statusChan)
	if err != nil {
		log.Fatalf("failed to solve llb: %v", err)
	}

	fmt.Println(green("Done!"))
}

func handleEnv(state llb.State, val string) llb.State {
	parts := strings.Split(val, "=")
	return state.AddEnv(parts[0], parts[1])
}

func handleCp(local llb.State, state llb.State, val string) llb.State {
	parts := strings.Split(val, " ")
	return state.File(llb.Copy(local, parts[0], parts[1], &llb.CopyInfo{}))
}

func handleExecute(state llb.State, op string) llb.State {
	return state.Run(llb.Shlex(op)).Root()
}
