package app

import (
	"log"
	"net"
	"net/url"
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Env struct {
	ENV  string
	HOST string
	PORT uint16
}

const (
	defaultEnv  = "dev"
	defaultHost = "0.0.0.0"
	defaultPort = 5001
)

func LoadEnv() *Env {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	env := &Env{
		ENV:  defaultEnv,
		HOST: defaultHost,
		PORT: defaultPort,
	}

	if e, ok := os.LookupEnv("ENV"); ok {
		if e != "dev" && e != "prod" {
			log.Fatalf("Invalid ENV: %s", e)
		}
		env.ENV = e
	}

	if host, ok := os.LookupEnv("HOST"); ok {
		if url, err := url.Parse(host); err == nil && url.Host != "" {
			env.HOST = url.String()
		} else if ip := net.ParseIP(host); ip != nil {
			env.HOST = ip.String()
		} else if host == "localhost" {
			env.HOST = "0.0.0.0"
		} else {
			log.Fatalf("Invalid HOST: %s", host)
		}
	}

	if n, err := strconv.ParseUint(os.Getenv("PORT"), 10, 16); err == nil {
		env.PORT = uint16(n)
	}

	return env
}
