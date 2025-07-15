package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/textproto"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

type Stage struct {
	Name   string                 `json:"Name"`
	Time   time.Time              `json:"Time"`
	Values map[string]interface{} `json:"Values"`
}

type BufferedClientTrace struct {
	httptrace.ClientTrace
	stages []Stage
}

func newStage(name string, values map[string]interface{}) Stage {
	return Stage{
		Name:   name,
		Time:   time.Now(),
		Values: values,
	}
}

func NewBufferedClientTrace() *BufferedClientTrace {
	trace := &BufferedClientTrace{
		stages: make([]Stage, 0, 16),
	}

	trace.ClientTrace = httptrace.ClientTrace{
		GetConn: func(hostPort string) {
			trace.stages = append(trace.stages, newStage("GetConn", map[string]interface{}{
				"hostPort": hostPort,
			}))
		},
		GotConn: func(info httptrace.GotConnInfo) {
			trace.stages = append(trace.stages, newStage("GotConn", map[string]interface{}{
				"GotConnInfo": info,
			}))
		},
		PutIdleConn: func(err error) {
			trace.stages = append(trace.stages, newStage("PutIdleConn", map[string]interface{}{
				"err": fmt.Sprintf("%v", err),
			}))
		},
		GotFirstResponseByte: func() {
			trace.stages = append(trace.stages, newStage("GotFirstResponseByte", map[string]interface{}{}))
		},
		Got100Continue: func() {
			trace.stages = append(trace.stages, newStage("Got100Continue", map[string]interface{}{}))
		},
		Got1xxResponse: func(code int, header textproto.MIMEHeader) error {
			trace.stages = append(trace.stages, newStage("Got1xxResponse", map[string]interface{}{
				"code":   code,
				"header": header,
			}))
			return nil
		},
		DNSStart: func(info httptrace.DNSStartInfo) {
			trace.stages = append(trace.stages, newStage("DNSStart", map[string]interface{}{
				"DNSStartInfo": info,
			}))
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			trace.stages = append(trace.stages, newStage("DNSDone", map[string]interface{}{
				"DNSDoneInfo": info,
			}))
		},
		ConnectStart: func(network, addr string) {
			trace.stages = append(trace.stages, newStage("ConnectStart", map[string]interface{}{
				"network": network,
				"addr":    addr,
			}))
		},
		ConnectDone: func(network, addr string, err error) {
			trace.stages = append(trace.stages, newStage("ConnectDone", map[string]interface{}{
				"network": network,
				"addr":    addr,
				"error":   err,
			}))
		},
		TLSHandshakeStart: func() {
			trace.stages = append(trace.stages, newStage("TLSHandshakeStart", map[string]interface{}{}))
		},
		TLSHandshakeDone: func(state tls.ConnectionState, err error) {
			trace.stages = append(trace.stages, newStage("TLSHandshakeDone", map[string]interface{}{
				"state": state,
				"error": err,
			}))
		},
		WroteHeaderField: func(key string, value []string) {
			trace.stages = append(trace.stages, newStage("WriteHeaderField", map[string]interface{}{
				"key":   key,
				"value": value,
			}))
		},
		WroteHeaders: func() {
			trace.stages = append(trace.stages, newStage("WriteHeaders", map[string]interface{}{}))
		},
		Wait100Continue: func() {
			trace.stages = append(trace.stages, newStage("Wait100Continue", map[string]interface{}{}))
		},
		WroteRequest: func(info httptrace.WroteRequestInfo) {
			trace.stages = append(trace.stages, newStage("WroteRequest", map[string]interface{}{
				"WroteRequestInfo": info,
			}))
		},
	}

	return trace
}

func doRequest(logger *logrus.Logger) bool {
	tlsConfig := tls.Config{}
	client := &http.Client{
		Transport: &http.Transport{
			Proxy:                  http.ProxyFromEnvironment,
			OnProxyConnectResponse: nil,
			TLSClientConfig:        &tlsConfig,
			TLSHandshakeTimeout:    10 * time.Second,
			IdleConnTimeout:        10 * time.Second,
			ResponseHeaderTimeout:  10 * time.Second,
			ExpectContinueTimeout:  10 * time.Second,
		},
		Timeout: 10 * time.Second,
	}

	trace := NewBufferedClientTrace()
	req, err := http.NewRequestWithContext(
		httptrace.WithClientTrace(context.Background(), &trace.ClientTrace),
		"GET",
		"https://update.traefik.io/repos/traefik/traefik/releases",
		nil)
	if err != nil {
		logger.WithError(err).Error("Error creating request")
		return false
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.WithError(err).WithField("stages", trace.stages).Error("Error requesting traefik releases")
		return true
	}
	defer resp.Body.Close()

	_, _ = io.Copy(io.Discard, resp.Body)
	logger.WithField("stages", trace.stages).Info("Requested traefik releases")

	return false
}

func doRequestAndCapture() bool {
	now := time.Now()

	logger := logrus.New()
	logger.SetLevel(logrus.DebugLevel)
	logger.SetFormatter(&logrus.JSONFormatter{})
	logFile, err := os.Create(fmt.Sprintf("out/%d-log.log", now.Unix()))
	if err != nil {
		logger.Fatal(err)
	}
	logger.SetOutput(logFile)
	defer logFile.Close()

	found := doRequest(logger)
	return found
}

func main() {
	_ = os.MkdirAll("out", 0755)

	fmt.Println("Capturing")
	for {
		fmt.Println("Trying HTTP request...")
		if doRequestAndCapture() {
			fmt.Println("connection error found!!!")
			break
		}
	}
}
