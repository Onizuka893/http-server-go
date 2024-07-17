package main

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

type httpRequest struct {
	requestLine requestLine
	requestBody requestBody
	headers     headers
}

type httpResponse struct {
	statusLine   statusLine
	responseBody requestBody
	headers      headers
}

type requestLine struct {
	method      method
	httpVersion string
}

type statusLine struct {
	status string
}

type method struct {
	methodType string
	methodUrl  string
}

type headers struct {
	host            string
	userAgent       string
	accept          string
	contentType     string
	acceptEncoding  string
	contentEncoding string
	contentLength   int
}

type requestBody struct {
	body string
}

var directoryPath string

func main() {
	args := os.Args
	if len(args) > 1 {
		directoryPath = args[2]
	}

	l, err := net.Listen("tcp", "0.0.0.0:4221")
	if err != nil {
		fmt.Println("Failed to bind to port 4221")
		os.Exit(1)
	}

	for {
		c, err := l.Accept()
		if err != nil {
			fmt.Println("Error accepting connection: ", err.Error())
			os.Exit(1)
		}
		go handleConnection(c)
	}
}

func handleConnection(c net.Conn) {
	request := make([]byte, 4096)

	length, err := c.Read(request)
	if length == 0 {
		fmt.Println("empty request", length)
	}
	if err != nil {
		fmt.Println("Error reading from connection", err.Error())
		return
	}
	httpRequest := httpReqParser(request)
	response := endpointMapper(httpRequest)

	_, err = c.Write([]byte(response))
	if err != nil {
		fmt.Println("Error writing to connection", err.Error())
		return
	}
}

func httpReqParser(req []byte) httpRequest {
	reqString := string(req)
	lines := strings.Split(reqString, "\r\n")

	methodline := strings.Split(lines[0], " ")
	method := method{methodType: methodline[0], methodUrl: methodline[1]}

	requestLine := requestLine{method: method, httpVersion: methodline[2]}
	headers := headers{}
	headerIndex := 1
	header := lines[headerIndex]
	for header != "" {
		headerSplit := strings.Split(header, " ")
		switch headerSplit[0] {
		case "Host:":
			headers.host = headerSplit[1]
		case "User-Agent:":
			headers.userAgent = headerSplit[1]
		case "Accept:":
			headers.accept = headerSplit[1]
		case "Content-Length:":
			contentLength, err := strconv.Atoi(headerSplit[1])
			if err != nil {
				fmt.Println("Failed to convert contentLength")
				break
			}
			headers.contentLength = contentLength
		case "Accept-Encoding:":
			headers.acceptEncoding = strings.Join(headerSplit[1:], "")
		}
		headerIndex++
		header = lines[headerIndex]
	}
	requestBody := requestBody{body: lines[headerIndex+1]}

	httpRequest := httpRequest{requestLine: requestLine, headers: headers, requestBody: requestBody}
	return httpRequest
}

func endpointMapper(httpRequest httpRequest) string {
	var response string
	endpoint := httpRequest.requestLine.method.methodUrl
	endpointSplit := strings.Split(endpoint, "/")
	if httpRequest.requestLine.method.methodType == "GET" {
		switch endpointSplit[1] {
		case "":
			response = "HTTP/1.1 200 OK\r\n\r\n"
		case "echo":
			response = echo(httpRequest)
		case "user-agent":
			response = userAgent(httpRequest)
		case "files":
			response = getFile(httpRequest)
		default:
			response = "HTTP/1.1 404 Not Found\r\n\r\n"
		}
	} else {
		switch endpointSplit[1] {
		case "files":
			response = postFile(httpRequest)
		default:
			response = "HTTP/1.1 404 Not Found\r\n\r\n"
		}
	}
	return response
}

func echo(httpRequest httpRequest) string {
	echo := strings.Split(httpRequest.requestLine.method.methodUrl, "/")[2]
	encoding := encodingChecker(httpRequest.headers.acceptEncoding)
	httpResponse := httpResponseBuilder(echo, "text/plain", "HTTP/1.1 200 OK", encoding)
	return httpResponseParser(httpResponse)
}

func userAgent(httpRequest httpRequest) string {
	useragent := httpRequest.headers.userAgent
	httpResponse := httpResponseBuilder(useragent, "text/plain", "HTTP/1.1 200 OK", encodingChecker(httpRequest.headers.acceptEncoding))
	return httpResponseParser(httpResponse)
}

func getFile(httpRequest httpRequest) string {
	fileName := strings.Split(httpRequest.requestLine.method.methodUrl, "/")[2]
	filePath := directoryPath + fileName
	dat, err := os.ReadFile(filePath)
	if err != nil {
		fmt.Println("Error reading file", err.Error())
		httpResponse := httpResponseBuilder("", "application/octet-stream", "HTTP/1.1 404 Not Found", encodingChecker(httpRequest.headers.acceptEncoding))
		return httpResponseParser(httpResponse)
	}
	httpResponse := httpResponseBuilder(string(dat), "application/octet-stream", "HTTP/1.1 200 OK", encodingChecker(httpRequest.headers.acceptEncoding))
	return httpResponseParser(httpResponse)
}

func postFile(httpRequest httpRequest) string {
	fileName := strings.Split(httpRequest.requestLine.method.methodUrl, "/")[2]
	content := httpRequest.requestBody.body
	contentLength := httpRequest.headers.contentLength
	filePath := directoryPath + fileName
	toWrite := make([]byte, contentLength)
	copy(toWrite, content)
	err := os.WriteFile(filePath, toWrite, 0644)
	if err != nil {
		fmt.Println("Error writing to file")
		return ""
	}
	return "HTTP/1.1 201 Created\r\n\r\n"
}

func httpResponseBuilder(body string, contentType string, status string, encoding string) httpResponse {
	if encoding != "" {
		return httpResponse{statusLine: statusLine{status: status}, responseBody: requestBody{body: body}, headers: headers{contentType: contentType, contentLength: len(body), contentEncoding: encoding}}
	}
	return httpResponse{statusLine: statusLine{status: status}, responseBody: requestBody{body: body}, headers: headers{contentType: contentType, contentLength: len(body)}}
}

func encodingChecker(encoding string) string {
	var acceptEncoding string
	encodingSlice := strings.Split(encoding, ",")
	for _, encode := range encodingSlice {
		switch encode {
		case "gzip":
			acceptEncoding += encode
		default:
		}
	}
	return acceptEncoding
}

func gzipCompression(data string) string {
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	w.Write([]byte(data))
	w.Close()
	return b.String()
}

func httpResponseParser(res httpResponse) string {
	var response string

	if res.statusLine.status != "HTTP/1.1 200 OK" {
		response = "HTTP/1.1 404 Not Found\r\n\r\n"
		return response
	}

	response += res.statusLine.status
	if res.headers.contentEncoding != "" {
		compressedBody := gzipCompression(res.responseBody.body)
		response += "\r\n"
		response += "Content-Encoding: "
		response += res.headers.contentEncoding
		response += "\r\n"
		response += "Content-Type: "
		response += res.headers.contentType
		response += "\r\n"
		response += "Content-Length: "
		response += strconv.Itoa(len(compressedBody))
		response += "\r\n"
		response += "\r\n"
		response += compressedBody
	} else {
		response += "\r\n"
		response += "Content-Type: "
		response += res.headers.contentType
		response += "\r\n"
		response += "Content-Length: "
		response += strconv.Itoa(res.headers.contentLength)
		response += "\r\n"
		response += "\r\n"
		response += res.responseBody.body
	}

	return response
}
