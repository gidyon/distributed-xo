package main

import (
	"flag"
	"github.com/Sirupsen/logrus"
	"github.com/gidyon/file-handlers/static"
	"github.com/gidyon/micros/pkg/conn"
	"net/http"
	"os"
)

const (
	siteCert   = "certs/cert.pem"
	siteKey    = "certs/key.pem"
	staticsDir = "dist"
)

func main() {
	var (
		certFile      = flag.String("cert", siteCert, "Path to public key")
		keyFile       = flag.String("key", siteKey, "Path to private key")
		redisHost     = flag.String("redis-host", "localhost", "Redis host")
		redisPort     = flag.String("redis-port", "6379", "Redis port")
		redisUser     = flag.String("redis-user", "root", "Redis host")
		redisSchema   = flag.String("redis-schema", "game", "Redis schema")
		redisPassword = flag.String("redis-password", "menevolent", "Redis password")
		env           = flag.Bool("env", false, "Whether to read parameters from env variables")
	)

	flag.Parse()

	if *env {
		*certFile = setIfEmpty(os.Getenv("TLS_CERT_FILE"), *certFile)
		*keyFile = setIfEmpty(os.Getenv("TLS_KEY_FILE"), *keyFile)

		*redisHost = setIfEmpty(os.Getenv("REDIS_HOST"), *redisHost)
		*redisPort = setIfEmpty(os.Getenv("REDIS_PORT"), *redisPort)
		*redisUser = setIfEmpty(os.Getenv("REDIS_USER"), *redisUser)
		*redisSchema = setIfEmpty(os.Getenv("REDIS_SCHEMA"), *redisSchema)
		*redisPassword = setIfEmpty(os.Getenv("REDIS_PASSWORD"), *redisPassword)
	}

	// open redis connection
	redisClient := conn.NewRedisClient(&conn.RedisOptions{
		Address: *redisHost,
		Port:    *redisPort,
	})

	// start game
	g, err := newGame(redisClient)
	if err != nil {
		logrus.Fatalln(err)
	}
	// static file server
	staticHandler, err := static.NewHandler(&static.ServerOptions{
		RootDir:       staticsDir,
		Index:         "index.html",
		FallBackIndex: true,
	})
	if err != nil {
		logrus.Fatalln(err)
	}

	server1 := http.Server{
		Handler: handler(g, staticHandler),
		Addr:    ":80",
	}

	// server2 := http.Server{
	// 	Handler: handler(g, staticHandler),
	// 	Addr:    ":443",
	// }

	// go logrus.Fatalln(server2.ListenAndServeTLS(*certFile, *keyFile))

	logrus.Infoln("server started on port 80(http) and 443(https)")
	logrus.Fatalln(server1.ListenAndServe())
}

func handler(g *game, staticHandler http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws", g.PlayerJoin)
	mux.Handle("/", staticHandler)
	return mux
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
