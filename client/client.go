package main

import (
	"bufio"
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const (
	SERVER_TYPE = "tcp"
	BUFFER_SIZE = 2048
)

type Student struct {
	Nama string
	Npm  string
}

type GreetResponse struct {
	Student Student
	Greeter string
}

type HttpRequest struct {
	Method         string
	Uri            string
	Version        string
	Host           string
	Accept         string
	AcceptEncoding string
}

type HttpResponse struct {
	Version         string
	StatusCode      string
	ContentType     string
	ContentEncoding string
	ContentLength   int
	Data            []byte
}

func main() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Input URL: ")
	inputURL, _ := reader.ReadString('\n')
	inputURL = strings.TrimSpace(inputURL)

	parsedURL, err := url.Parse(inputURL)
	if err != nil {
		fmt.Printf("Error parsing URL: %v\n", err)
		return
	}

	host := parsedURL.Hostname()
	port := parsedURL.Port()
	uri := parsedURL.Path

	if parsedURL.RawQuery != "" {
		uri += "?" + parsedURL.RawQuery
	}

	fmt.Print("Input Content Type: ")
	contentType, _ := reader.ReadString('\n')
	contentType = strings.TrimSpace(contentType)

	fmt.Print("Input Accept Encoding (write \"none\" if no special encoding can be accepted): ")
	acceptEncoding, _ := reader.ReadString('\n')
	acceptEncoding = strings.TrimSpace(acceptEncoding)

	httpReq := HttpRequest{
		Method:         "GET",
		Uri:            uri,
		Version:        "HTTP/1.1",
		Host:           host + ":" + port,
		Accept:         contentType,
		AcceptEncoding: acceptEncoding,
	}

	serverAddr := host + ":" + port
	connection, err := net.Dial(SERVER_TYPE, serverAddr)
	if err != nil {
		fmt.Printf("Error connecting to server: %v\n", err)
		return
	}
	defer connection.Close()

	response := Fetch(httpReq, connection)

	fmt.Printf("Status Code: %s\n", response.StatusCode)
	if response.ContentEncoding != "" && response.ContentEncoding != "none" {
		fmt.Printf("Encoded: %s\n", response.ContentEncoding)
	}

	decodedData := response.Data
	if response.ContentEncoding == "gzip" {
		decodedData = decompressGzip(response.Data)
	} else if response.ContentEncoding == "deflate" {
		decodedData = decompressDeflate(response.Data)
	}

	bodyStr := strings.TrimSpace(string(decodedData))
	fmt.Printf("Body: %s\n", bodyStr)

	if len(decodedData) > 0 {
		var greetResponse GreetResponse
		var err error

		if strings.Contains(response.ContentType, "application/json") {
			err = json.Unmarshal(decodedData, &greetResponse)
		} else if strings.Contains(response.ContentType, "application/xml") {
			err = xml.Unmarshal(decodedData, &greetResponse)
		}

		if err == nil && (strings.Contains(response.ContentType, "application/json") || strings.Contains(response.ContentType, "application/xml")) {
			fmt.Printf("Parsed: %v\n", greetResponse)
		}
	}
}

func Fetch(req HttpRequest, connection net.Conn) HttpResponse {
	requestBytes := RequestEncoder(req)

	_, err := connection.Write(requestBytes)
	if err != nil {
		fmt.Printf("Error sending request: %v\n", err)
		return HttpResponse{}
	}

	buffer := make([]byte, BUFFER_SIZE)
	var responseData []byte

	for {
		n, err := connection.Read(buffer)
		if err != nil {
			if n == 0 {
				break
			}
			fmt.Printf("Error reading response: %v\n", err)
			break
		}
		responseData = append(responseData, buffer[:n]...)

		responseStr := string(responseData)
		if strings.Contains(responseStr, "\r\n\r\n") {
			headerEndIndex := strings.Index(responseStr, "\r\n\r\n")
			headers := responseStr[:headerEndIndex]

			contentLength := 0
			headerLines := strings.Split(headers, "\r\n")
			for _, line := range headerLines {
				if strings.HasPrefix(strings.ToLower(line), "content-length:") {
					parts := strings.SplitN(line, ":", 2)
					if len(parts) == 2 {
						if length, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
							contentLength = length
						}
					}
					break
				}
			}

			bodyStart := headerEndIndex + 4
			currentBodyLength := len(responseData) - bodyStart

			if contentLength == 0 || currentBodyLength >= contentLength {
				break
			}
		}

		if n < BUFFER_SIZE {
			break
		}
	}

	return ResponseDecoder(responseData)
}

func ResponseDecoder(bytestream []byte) HttpResponse {
	responseStr := string(bytestream)
	lines := strings.Split(responseStr, "\r\n")

	response := HttpResponse{}

	if len(lines) > 0 {
		statusParts := strings.Split(lines[0], " ")
		if len(statusParts) >= 3 {
			response.Version = statusParts[0]
			response.StatusCode = statusParts[1]
		}
	}

	headerEndIndex := 0
	for i, line := range lines {
		if line == "" {
			headerEndIndex = i
			break
		}

		if i == 0 {
			continue
		}

		headerParts := strings.SplitN(line, ": ", 2)
		if len(headerParts) == 2 {
			headerName := strings.ToLower(headerParts[0])
			headerValue := headerParts[1]

			switch headerName {
			case "content-type":
				response.ContentType = headerValue
			case "content-encoding":
				response.ContentEncoding = headerValue
			case "content-length":
				if length, err := strconv.Atoi(headerValue); err == nil {
					response.ContentLength = length
				}
			}
		}
	}

	if headerEndIndex < len(lines)-1 {
		bodyLines := lines[headerEndIndex+1:]
		body := strings.Join(bodyLines, "\r\n")
		response.Data = []byte(body)
	}

	return response
}

func RequestEncoder(req HttpRequest) []byte {
	var requestBuilder strings.Builder

	requestBuilder.WriteString(fmt.Sprintf("%s %s %s\r\n", req.Method, req.Uri, req.Version))

	requestBuilder.WriteString(fmt.Sprintf("Host: %s\r\n", req.Host))

	requestBuilder.WriteString(fmt.Sprintf("Accept: %s\r\n", req.Accept))

	if req.AcceptEncoding != "none" {
		requestBuilder.WriteString(fmt.Sprintf("Accept-Encoding: %s\r\n", req.AcceptEncoding))
	}

	requestBuilder.WriteString("\r\n")

	return []byte(requestBuilder.String())
}

func decompressGzip(data []byte) []byte {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		fmt.Printf("Error creating gzip reader: %v\n", err)
		return data
	}
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		fmt.Printf("Error decompressing gzip data: %v\n", err)
		return data
	}

	return decompressed
}

func decompressDeflate(data []byte) []byte {
	reader := flate.NewReader(bytes.NewReader(data))
	defer reader.Close()

	decompressed, err := io.ReadAll(reader)
	if err != nil {
		fmt.Printf("Error decompressing deflate data: %v\n", err)
		return data
	}

	return decompressed
}
