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

func handleInit() {
	config := godevwatch.DefaultConfig()
	if err := config.Save("godevwatch.yaml"); err != nil {
		log.Fatalf("Failed to create config file: %v", err)
	}
	fmt.Println("Created godevwatch.yaml")
}

func main() {
	// Handle init subcommand before flag parsing
	if len(os.Args) > 1 && os.Args[1] == "init" {
		handleInit()
		return
	}

	// Parse flags for main command
	var (
		configPath   = flag.String("config", "", "Path to configuration file")
		proxyPort    = flag.Int("proxy-port", 3000, "Proxy server port")
		backendPort  = flag.Int("backend-port", 8080, "Backend server port")
		statusDir    = flag.String("status-dir", "tmp/.build-status", "Build status directory")
		injectScript = flag.Bool("inject-script", true, "Inject live reload script into HTML responses")
		showVersion  = flag.Bool("version", false, "Show version information")
	)

	flag.Parse()

	if *showVersion {
		fmt.Printf("godevwatch %s (commit: %s, built: %s)\n", version, commit, date)
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

	// Enable watch mode if build rules and run command are configured
	enableWatch := len(config.BuildRules) > 0 && config.RunCmd != ""

	if !enableWatch {
		if len(config.BuildRules) == 0 && config.RunCmd == "" {
			log.Println("File watching disabled: build_rules and run_cmd are not configured")
		} else if len(config.BuildRules) == 0 {
			log.Println("File watching disabled: build_rules are not configured")
		} else {
			log.Println("File watching disabled: run_cmd is not configured")
		}
		log.Println("To enable file watching, configure both build_rules and run_cmd in your config file")
	}

	// Clean up any processes on the ports we're going to use
	log.Println("Checking for existing processes on configured ports...")
	if err := godevwatch.KillProcessOnPort(config.ProxyPort); err != nil {
		log.Printf("Warning: Failed to clean up proxy port: %v", err)
	}
	if err := godevwatch.KillProcessOnPort(config.BackendPort); err != nil {
		log.Printf("Warning: Failed to clean up backend port: %v", err)
	}

	// Create build tracker
	buildTracker := godevwatch.NewBuildTracker(config.BuildStatusDir)

	// Create file watcher if enabled
	var watcher *godevwatch.FileWatcher
	if enableWatch {
		watcher, err = godevwatch.NewFileWatcher(config, buildTracker)
		if err != nil {
			log.Fatalf("Failed to create file watcher: %v", err)
		}
		defer watcher.Stop()

		if err := watcher.Start(); err != nil {
			log.Fatalf("Failed to start file watcher: %v", err)
		}
	}

	// Create proxy server with shared build tracker
	proxy, err := godevwatch.NewProxyServer(config, buildTracker)
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
		if watcher != nil {
			watcher.Stop()
		}
		proxy.Close()
		os.Exit(0)
	}()

	// Start proxy server
	if err := proxy.Start(); err != nil {
		log.Fatalf("Proxy server error: %v", err)
	}
}
