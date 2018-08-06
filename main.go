// Package main contains the main functionality of the lander application.
package main

import (
	"errors"
	"github.com/fsouza/go-dockerclient"
	log "github.com/sirupsen/logrus"
	"html/template"
	"net/http"
	"os"
	"strings"
)

// Type Config stores all configuration needed for lander
type Config struct {
	Traefik  string // should be a bool, but for now it's okay the way it is; determines whether lander searches for traefik labels. Default: true
	Exposed  string // should be a bool, but for now it's okay the way it is; determines whether lander searches for exposed ports. Default: false
	Listen   string // the ip and port on which lander will listen in the format <IP>:PORT. Default: :8080
	Title    string // the title displayed on top of the default template header. Default: LANDER
	Hostname string // the hostname of the host machine, used to create hyperlinks. Default: ""
	Docker   string // path to docker's api endpoint (e.g. unix:///var/run/docker.sock)
}

// Type Container stores the name of the application (running in an container) and the corresponding url
type Container struct {
	AppName string // name of the application. Will be displayed as link title in the rendered template
	AppURL  string // url (or better the context) of the application. Will be used to create hyperlinks
}

// Type PayloadData holds the title of the future index.html and a map of slices of struct Container
type PayloadData struct {
	Title  string                 // the title displayed on top of the default template. must be in here so that we can pass one big struct to the html-template renderer
	Groups map[string][]Container // map of container groups. used to group the applications in the rendered template/for headers of the html table rows
}

var RuntimeConfig Config

// Get is a method on variables from type PayloadData which gets all available metadata.
func (payload PayloadData) Get(containers []docker.APIContainers) {
	// iterate through slice of containers and find "lander" labels
	for _, container := range containers {
		// check if map contains a key named "lander.enable"
		if _, found := container.Labels["lander.enable"]; found {
			// give debug messages
			log.Debug("found lander labels on Container: ", container.ID)

			containerName, containerURL, err := GetTraefikConfiguration(container)
			if err != nil {
				continue
			}
			//if RuntimeConfig.Exposed == "true" {
			//containerName, containerURL := GetExposedConfiguration(container)
			//}

			// check if lander.group is already present
			if _, found := payload.Groups[container.Labels["lander.group"]]; found {
				payload.Groups[container.Labels["lander.group"]] = append(payload.Groups[container.Labels["lander.group"]], Container{AppName: containerName, AppURL: containerURL})
			} else {
				payload.Groups[container.Labels["lander.group"]] = []Container{Container{AppName: containerName, AppURL: containerURL}}
			}
		}
	}
}

func GetTraefikConfiguration(container docker.APIContainers) (containerName string, containerURL string, err error) {
	if RuntimeConfig.Traefik == "true" {
		// extract strings for easier use
		containerName := container.Labels["lander.name"]
		delimiterPosition := strings.LastIndex(container.Labels["traefik.frontend.rule"], ":")
		containerURL := container.Labels["traefik.frontend.rule"][delimiterPosition:]
		// return extracted values
		return containerName, containerURL, nil
	} else {
		err := errors.New("LANDER_TRAEFIK is set to false")
		return "", "", err
	}
}

// GetContainers
func GetContainers(dockerSocket string) []docker.APIContainers {
	// get new client
	client, err := docker.NewClient(dockerSocket)
	if err != nil {
		log.Panic(err)
	}

	// get running containers
	containers, err := client.ListContainers(docker.ListContainersOptions{All: true})
	if err != nil {
		log.Panic(err)
	}

	return containers
}

// RenderAndRespond get's the metadata to render, renders and delivers the http GET response.
func RenderAndRespond(w http.ResponseWriter, r *http.Request) {
	// check if the request is exactly "/", otherwise stop the response
	if r.URL.String() != "/" {
		log.Error(r.RemoteAddr, " ", r.URL, " not a valid request")
		return
	}

	// print request to log
	log.Debug(r.RemoteAddr, " ", r.Method, " ", r.URL)

	var payload = PayloadData{"", make(map[string][]Container)}
	payload.Get(GetContainers(RuntimeConfig.Docker))

	payload.Title = RuntimeConfig.Title

	templ := template.Must(template.ParseFiles("template.html"))

	err := templ.Execute(w, payload)
	if err != nil {
		log.Panic(err)
	}
}

// GetConfig reads the configuration from system environment variables or sets a default value.
func GetConfig() Config {
	// create new variable of type Config
	var config Config

	// try to get the path to docker's socket and exit the application if not found
	config.Docker = os.Getenv("LANDER_DOCKER")
	if config.Docker == "" {
		// throw a fatal-message into log and quit the application, since we can't do anything useful without a docker daemon to connect to
		log.Fatal("environment variable LANDER_DOCKER not set! Can't start the server without a docker endpoint.")
	}

	// try to get the value of ENV "LANDER_TRAEFIK" and set a default value if not successful
	config.Traefik = os.Getenv("LANDER_TRAEFIK")
	if config.Traefik == "" {
		log.Info("environment variable LANDER_TRAEFIK not set, assuming: \"true\"")
		config.Traefik = "true"
	}

	// try to get the value of ENV "LANDER_EXPOSED" and set a default value if not successful
	config.Exposed = os.Getenv("LANDER_EXPOSED")
	if config.Exposed == "" {
		log.Info("environment variable LANDER_EXPOSED not set, assuming: \"false\"")
		config.Exposed = "false"
	}

	// try to get the value of ENV "LANDER_LISTEN" and set a default value if not successful
	config.Listen = os.Getenv("LANDER_LISTEN")
	if config.Listen == "" {
		log.Info("environment variable LANDER_LISTEN not set, assuming: \"8080\"")
		config.Listen = ":8080"
	}

	// try to get the value of ENV "LANDER_TITLE" and set a default value if not successful
	config.Title = os.Getenv("LANDER_TITLE")
	if config.Title == "" {
		log.Info("environment variable LANDER_TITLE not set, assuming: \"LANDER\"")
		config.Title = "LANDER"
	}

	// try to get the value of ENV "LANDER_HOSTNAME" and set a default value if not successful
	config.Hostname = os.Getenv("LANDER_HOSTNAME")
	if config.Hostname == "" {
		log.Warn("environment variable LANDER_HOSTNAME not set! We might not be able to generate valid hyperlinks!")
		config.Hostname = ""
	}

	return config
}

func initLogger() {
	RequestedLogLevel := os.Getenv("LANDER_LOGLEVEL")
	if RequestedLogLevel != "" {
		switch RequestedLogLevel {
		case "info":
			log.SetLevel(log.InfoLevel)
		case "debug":
			log.SetLevel(log.DebugLevel)
		case "warn":
			log.SetLevel(log.WarnLevel)
		case "panic":
			log.SetLevel(log.PanicLevel)
		case "fatal":
			log.SetLevel(log.FatalLevel)
		}
	}
}

func startHTTPListener() {
	// register handle function for root context
	http.HandleFunc("/", RenderAndRespond)

	// start listener
	log.Info("Starting Server on ", RuntimeConfig.Listen)
	err := http.ListenAndServe(RuntimeConfig.Listen, nil)
	if err != nil {
		log.Panic(err)
	}
}

func main() {
	// get configuration
	// initialize logger and start listener
	RuntimeConfig = GetConfig()
	initLogger()
	startHTTPListener()
}
