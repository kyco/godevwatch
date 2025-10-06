package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/kyco/godevwatch"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	var (
		configPath   = flag.String("config", "", "Path to configuration file")
		proxyPort    = flag.Int("proxy-port", 3000, "Proxy server port")
		backendPort  = flag.Int("backend-port", 8080, "Backend server port")
		statusDir    = flag.String("status-dir", "tmp/.build-counters", "Build status directory")
		injectScript = flag.Bool("inject-script", true, "Inject live reload script into HTML responses")
		initConfig   = flag.Bool("init", false, "Create a default configuration file")
		showVersion  = flag.Bool("version", false, "Show version information")
	)

	flag.Parse()

	if *showVersion {
		fmt.Printf("godevwatch %s (commit: %s, built: %s)\n", version, commit, date)
		os.Exit(0)
	}

	if *initConfig {
		config := godevwatch.DefaultConfig()
		if err := config.Save("godevwatch.yaml"); err != nil {
			log.Fatalf("Failed to create config file: %v", err)
		}
		fmt.Println("Created godevwatch.yaml")
		os.Exit(0)
	}

	var config *godevwatch.Config
	var err error

	if *configPath != "" {
		config, err = godevwatch.LoadConfig(*configPath)
		if err != nil {
			log.Fatalf("Failed to load config: %v", err)
		}
	} else {
		// Check for default config file
		if _, err := os.Stat("godevwatch.yaml"); err == nil {
			config, err = godevwatch.LoadConfig("godevwatch.yaml")
			if err != nil {
				log.Fatalf("Failed to load config: %v", err)
			}
		} else {
			// Use default config with CLI flags
			config = godevwatch.DefaultConfig()
			config.ProxyPort = *proxyPort
			config.BackendPort = *backendPort
			config.BuildStatusDir = *statusDir
			config.InjectScript = *injectScript
		}
	}

	// Create proxy server
	proxy, err := godevwatch.NewProxyServer(config)
	if err != nil {
		log.Fatalf("Failed to create proxy server: %v", err)
	}
	defer proxy.Close()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\nShutting down gracefully...")
		proxy.Close()
		os.Exit(0)
	}()

	// Start proxy server
	if err := proxy.Start(); err != nil {
		log.Fatalf("Proxy server error: %v", err)
	}
}
