package main

import (
    "github.com/oschwald/maxminddb-golang"
    log "github.com/sirupsen/logrus"
    "github.com/spf13/viper"
    "net"
    "net/http"
    "net/url"
    "strings"
)

var (
    db *maxminddb.Reader
    settings = &Settings{}
)

type City struct {
    Location struct {
        Latitude       float64 `maxminddb:"latitude"`
        Longitude      float64 `maxminddb:"longitude"`
    } `maxminddb:"location"`
}

type Settings struct {
    Servers ServerList
}

func main() {
    viper.SetConfigName("dlrouter") // name of config file (without extension)
    viper.SetConfigType("yaml") // REQUIRED if the config file does not have the extension in the name
    viper.AddConfigPath("/etc/dlrouter/")   // path to look for the config file in
    viper.AddConfigPath("$HOME/.dlrouter")  // call multiple times to add many search paths
    viper.AddConfigPath(".")               // optionally look for config in the working directory
    err := viper.ReadInConfig() // Find and read the config file

    if err != nil { // Handle errors reading the config file
        log.WithError(err).Fatalln("Unable to load config file")
    }

    db, err = maxminddb.Open(viper.GetString("geodb"))

    if err != nil {
        log.WithError(err).Fatalln("Unable to open database")
    }

    servers := viper.GetStringSlice("servers")

    for _, server := range servers {
        var prefix string

        if !strings.HasPrefix(server, "http") {
            prefix = "https://"
        }

        u, err := url.Parse(prefix + server)

        if err != nil {
            log.WithFields(log.Fields{
                "error": err,
                "server": server,
            }).Warning("Server is invalid")
            continue
        }

        ips, err := net.LookupIP(u.Host)

        if err != nil {
            log.WithFields(log.Fields{
                "error": err,
                "server": server,
            }).Warning("Could not resolve address")
            continue
        }

        var city City
        err = db.Lookup(ips[0], &city)

        if err != nil {
            log.WithFields(log.Fields{
                "error": err,
                "server": server,
            }).Warning("Could not geolocate address")
            continue
        }

        settings.Servers = append(settings.Servers, &Server{
            Host: u.Host,
            Path: u.Path,
            Latitude: city.Location.Latitude,
            Longitude: city.Location.Longitude,
        })

        log.WithFields(log.Fields{
            "server": u.Host,
            "path": u.Path,
            "latitude": city.Location.Latitude,
            "longitude": city.Location.Longitude,
        }).Info("Added server")
    }

    log.Info("Servers added, checking statuses")
    // Force initial check before running
    settings.Servers.Check()

    // Start check loop
    go settings.Servers.checkLoop()

    log.Info("Starting")

    mux := http.NewServeMux()

    mux.HandleFunc("/status", RealIPMiddleware(statusRequest))
    mux.HandleFunc("/", RealIPMiddleware(redirectRequest))

    http.ListenAndServe(":8080", mux)
}
