package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/procfs"
)

type blocksLatest struct {
	Block innerBlock `json:"block"`
}

type innerBlock struct {
	Header headerData `json:"header"`
}

type headerData struct {
	Height string `json:"height"`
	Time   string `json:"time"`
}

type addrBookJson struct {
	Addrs []addrBook `json:"addrs"`
}

type addrBook struct {
	Addr addrData `json:"addr"`
}

type addrData struct {
	Ip string `json:"ip"`
}

var addr = flag.String("listen-address", ":8080", "The address to listen on for HTTP requests.")
var configPath = flag.String("config-path", "/root/.gaia/config/addrbook.json", "Path to gaiad config")
var appHost = flag.String("app-host", "127.0.0.1", "Host of exposed API")
var appPort = flag.String("app-port", ":1317", "Port of exposed API")

func customHandler(prom http.Handler) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {

		prom.ServeHTTP(w, r)

		var wg sync.WaitGroup
		wg.Add(3)

		go callApi(*appHost+*appPort, "block", blockNum, &wg)
		go callApi(*appHost+*appPort, "time", timeSkew, &wg)
		go getPeerAmount(*configPath, peerAmount, &wg)

		wg.Wait()

	}

	return http.HandlerFunc(fn)
}

func intersection(remote, book []net.IP) (common []string) {

	saveCurIps := make(map[string]bool)

	for _, item := range remote {
		saveCurIps[item.String()] = true
	}

	for _, item := range book {
		if _, ok := saveCurIps[item.String()]; ok {
			common = append(common, item.String())
		}
	}
	return
}

func callApi(apiHost string, callType string, metric prometheus.Gauge, wg *sync.WaitGroup) {
	defer wg.Done()

	log.Println("Calling ", callType)

	apiRoute := "blocks/latest"

	fullApiRoute := fmt.Sprintf("http://%s/%s", apiHost, apiRoute)
	response, err := http.Get(fullApiRoute)
	if err != nil {
		log.Printf("%s", err)
	} else {
		contents, err := ioutil.ReadAll(response.Body)
		if err != nil {
			log.Printf("%s", err)
		}

		apiResponse := blocksLatest{}
		json.Unmarshal([]byte(contents), &apiResponse)

		if callType == "block" {
			height, _ := strconv.ParseFloat(apiResponse.Block.Header.Height, 64)

			metric.Set(height)
		} else if callType == "time" {
			blockDateApi := apiResponse.Block.Header.Time

			blockDate, _ := time.Parse(time.RFC3339Nano, blockDateApi)
			blockSec := blockDate.UnixNano()

			now := time.Now()
			curNanoSec := now.UnixNano()

			metric.Set(float64(curNanoSec - blockSec))
		}

		response.Body.Close()
	}

}

func getPeerAmount(addrBook string, metric prometheus.Gauge, wg *sync.WaitGroup) {
	log.Println("Calling peers")

	defer wg.Done()

	var remoteIps []net.IP
	var addrbookIps []net.IP

	fs, err := procfs.NewDefaultFS()
	if err != nil {
		log.Println(err)
	}

	ct, err := fs.NetTCP()
	if err != nil {
		log.Println(err)
	}

	for _, conn := range ct {
		if conn.RemPort == 26656 {
			remoteIps = append(remoteIps, conn.RemAddr)
		}
	}

	jsonFile, err := os.Open(addrBook)
	if err != nil {
		log.Println(err)
	}
	addrJsonBytes, err := ioutil.ReadAll(jsonFile)
	if err != nil {
		log.Println(err)
	}
	addrJson := addrBookJson{}
	json.Unmarshal([]byte(addrJsonBytes), &addrJson)
	jsonFile.Close()

	for _, addr := range addrJson.Addrs {
		addrIp, _, _ := net.ParseCIDR(addr.Addr.Ip + "/32")
		addrbookIps = append(addrbookIps, addrIp)
	}

	commonIps := intersection(remoteIps, addrbookIps)

	metric.Set(float64(len(commonIps)))

}

var (
	blockNum = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cosmos_block_number",
		Help: "Number of the latest block in chain",
	})
	timeSkew = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cosmos_block_time_skew",
		Help: "Difference between current timestamp and block timestamp",
	})
	peerAmount = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "cosmos_node_peers",
		Help: "Amount of chain peers",
	})
)

func main() {
	flag.Parse()

	prometheus.MustRegister(blockNum)
	prometheus.MustRegister(timeSkew)
	prometheus.MustRegister(peerAmount)

	promHandler := promhttp.Handler()

	http.Handle("/metrics", customHandler(promHandler))
	log.Printf("Starting web server at %s\n", *addr)

	err := http.ListenAndServe(*addr, nil)
	if err != nil {
		log.Fatal("http.ListenAndServer: %v\n", err)
	}

}
