package main

import (
	"flag"
	"github.com/Sirupsen/logrus"
	"github.com/gidyon/file-handlers/static"
	"github.com/gidyon/micros/pkg/conn"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const (
	siteCert = "certs/cert.pem"
	siteKey  = "certs/key.pem"
	rootDir  = "dist"
)

func main() {
	var (
		certFile      = flag.String("cert", siteCert, "Path to public key")
		keyFile       = flag.String("key", siteKey, "Path to private key")
		port          = flag.String("port", ":443", "Port to serve files")
		root          = flag.String("root", rootDir, "Root directory")
		redisHost     = flag.String("redis-host", "localhost", "Redis host")
		redisPort     = flag.String("redis-port", "6379", "Redis port")
		redisUser     = flag.String("redis-user", "root", "Redis host")
		redisSchema   = flag.String("redis-schema", "game", "Redis schema")
		redisPassword = flag.String("redis-password", "menevolent", "Redis password")
		flush         = flag.Bool("flush", false, "Whether to flush all data in redis")
		env           = flag.Bool("env", false, "Whether to read parameters from env variables")
	)

	flag.Parse()

	if *env {
		*certFile = setIfEmpty(os.Getenv("TLS_CERT_FILE"), *certFile)
		*keyFile = setIfEmpty(os.Getenv("TLS_KEY_FILE"), *keyFile)
		*port = setIfEmpty(os.Getenv("PORT"), *port)
		*root = setIfEmpty(os.Getenv("ROOT_DIR"), *root)

		*redisHost = setIfEmpty(os.Getenv("REDIS_HOST"), *redisHost)
		*redisPort = setIfEmpty(os.Getenv("REDIS_PORT"), *redisPort)
		*redisUser = setIfEmpty(os.Getenv("REDIS_USER"), *redisUser)
		*redisSchema = setIfEmpty(os.Getenv("REDIS_SCHEMA"), *redisSchema)
		*redisPassword = setIfEmpty(os.Getenv("REDIS_PASSWORD"), *redisPassword)
		*flush, _ = strconv.ParseBool(os.Getenv("FLUSH_REDIS"))
	}

	// open redis connection
	redisClient := conn.NewRedisClient(&conn.RedisOptions{
		Address: *redisHost,
		Port:    *redisPort,
	})

	if *flush {
		defer logrus.Errorln(redisClient.FlushAll().Err())
	}

	// start game
	g, err := newGame(redisClient)
	if err != nil {
		logrus.Fatalln(err)
	}
	http.HandleFunc("/ws", g.PlayerJoin)

	// static file server
	staticHandler, err := static.NewHandler(&static.ServerOptions{
		RootDir: *root,
		Index:   "index.html",
	})
	if err != nil {
		logrus.Fatalln(err)
	}
	http.Handle("/", staticHandler)

	logrus.Infof("server started on %v\n", *port)

	*port = ":" + strings.TrimPrefix(*port, ":")
	logrus.Fatalln(http.ListenAndServeTLS(*port, *certFile, *keyFile, nil))
}

func setIfEmpty(strCurrent, strFinal string) string {
	if strCurrent == "" && strFinal == "" {
		return ""
	}
	if strCurrent == "" {
		return strFinal
	}
	return strCurrent
}
