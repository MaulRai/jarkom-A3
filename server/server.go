package main

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net"
	"net/url"
	"strings"
)

const (
	SERVER_HOST  = "127.0.0.1"
	SERVER_PORT  = "6636"
	SERVER_TYPE  = "tcp"
	BUFFER_SIZE  = 2048
	STUDENT_NAME = "Muhammad Raihan Maulana"
	STUDENT_NPM  = "2306216636"
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
	// Create TCP listener on the specified port
	listener, err := net.Listen(SERVER_TYPE, SERVER_HOST+":"+SERVER_PORT)
	if err != nil {
		fmt.Printf("Error starting server: %v\n", err)
		return
	}
	defer listener.Close()

	fmt.Printf("Server listening on %s:%s\n", SERVER_HOST, SERVER_PORT)

	// Handle multiple connections
	for {
		connection, err := listener.Accept()
		if err != nil {
			fmt.Printf("Error accepting connection: %v\n", err)
			continue
		}

		// Handle each connection in a separate goroutine
		go HandleConnection(connection)
	}
}

func HandleConnection(connection net.Conn) {
	defer connection.Close()
	
	// Read request from client
	buffer := make([]byte, BUFFER_SIZE)
	var requestData []byte
	
	for {
		n, err := connection.Read(buffer)
		if err != nil {
			if n == 0 {
				break
			}
			fmt.Printf("Error reading request: %v\n", err)
			return
		}
		
		requestData = append(requestData, buffer[:n]...)
		
		// Check if we have received the complete request
		requestStr := string(requestData)
		if strings.Contains(requestStr, "\r\n\r\n") {
			break // Complete request received
		}
		
		if n < BUFFER_SIZE {
			break
		}
	}
	
	// Decode the request
	httpReq := RequestDecoder(requestData)
	
	// Handle the request and generate response
	httpRes := HandleRequest(httpReq)
	
	// Encode and send response
	responseBytes := ResponseEncoder(httpRes)
	connection.Write(responseBytes)
}

func HandleRequest(req HttpRequest) HttpResponse {
	// Parse URI to extract path and query parameters
	parsedURL, err := url.Parse(req.Uri)
	if err != nil {
		return HttpResponse{
			Version:    "HTTP/1.1",
			StatusCode: "400",
		}
	}

	path := parsedURL.Path
	query := parsedURL.Query()

	switch path {
	case "/":
		return handleRoot(req)
	default:
		if strings.HasPrefix(path, "/greet/") {
			return handleGreet(req, path, query)
		}
		return handle404()
	}
}

func handleRoot(req HttpRequest) HttpResponse {
	htmlContent := fmt.Sprintf("<html><body><h1>Halo, dunia! Aku %s sedang mengerjakan A03</h1></body></html>", STUDENT_NAME)
	
	response := HttpResponse{
		Version:         "HTTP/1.1",
		StatusCode:      "200",
		ContentType:     "text/html",
		ContentEncoding: "none",
		Data:            []byte(htmlContent),
	}
	
	response.ContentLength = len(response.Data)
	return response
}

func handleGreet(req HttpRequest, path string, query url.Values) HttpResponse {
	// Extract NPM from path
	parts := strings.Split(path, "/")
	if len(parts) < 3 {
		return handle404()
	}
	
	npm := parts[2]
	if npm != STUDENT_NPM {
		return handle404()
	}

	// Get greeter name from query parameter or use student name as default
	greeterName := STUDENT_NAME
	if nameParam := query.Get("name"); nameParam != "" {
		greeterName = nameParam
	}

	// Create response data
	student := Student{
		Nama: STUDENT_NAME,
		Npm:  STUDENT_NPM,
	}
	
	greetResponse := GreetResponse{
		Student: student,
		Greeter: greeterName,
	}

	// Determine content type based on Accept header
	contentType := determineContentType(req.Accept)
	
	var responseData []byte
	var err error
	
	if contentType == "application/xml" {
		responseData, err = xml.Marshal(greetResponse)
	} else {
		// Default to JSON
		contentType = "application/json"
		responseData, err = json.Marshal(greetResponse)
	}
	
	if err != nil {
		return HttpResponse{
			Version:    "HTTP/1.1",
			StatusCode: "500",
		}
	}

	// Determine encoding
	encoding := determineEncoding(req.AcceptEncoding)
	
	// Apply compression if needed
	if encoding == "gzip" {
		responseData = compressGzip(responseData)
	} else if encoding == "deflate" {
		responseData = compressDeflate(responseData)
	} else {
		encoding = "none"
	}

	response := HttpResponse{
		Version:         "HTTP/1.1",
		StatusCode:      "200",
		ContentType:     contentType,
		ContentEncoding: encoding,
		Data:            responseData,
	}
	
	response.ContentLength = len(response.Data)
	return response
}

func handle404() HttpResponse {
	return HttpResponse{
		Version:    "HTTP/1.1",
		StatusCode: "404",
	}
}

func determineContentType(accept string) string {
	accept = strings.ToLower(accept)
	
	// If multiple types or q values present, default to JSON
	if strings.Contains(accept, ",") || strings.Contains(accept, "q=") {
		return "application/json"
	}
	
	// Check for specific types
	if strings.Contains(accept, "application/xml") {
		return "application/xml"
	} else if strings.Contains(accept, "application/json") {
		return "application/json"
	}
	
	// Default to JSON for any other type
	return "application/json"
}

func determineEncoding(acceptEncoding string) string {
	acceptEncoding = strings.ToLower(acceptEncoding)
	
	// If multiple encodings or q values present, default to gzip
	if strings.Contains(acceptEncoding, ",") || strings.Contains(acceptEncoding, "q=") {
		return "gzip"
	}
	
	// Check for specific encodings
	if strings.Contains(acceptEncoding, "deflate") {
		return "deflate"
	} else if strings.Contains(acceptEncoding, "gzip") {
		return "gzip"
	} else if acceptEncoding == "none" {
		return "none"
	}
	
	// For any other encoding, default to gzip
	return "gzip"
}

func RequestDecoder(bytestream []byte) HttpRequest {
	requestStr := string(bytestream)
	lines := strings.Split(requestStr, "\r\n")
	
	req := HttpRequest{}
	
	// Parse request line (Method URI Version)
	if len(lines) > 0 {
		requestLineParts := strings.Split(lines[0], " ")
		if len(requestLineParts) >= 3 {
			req.Method = requestLineParts[0]
			req.Uri = requestLineParts[1]
			req.Version = requestLineParts[2]
		}
	}
	
	// Parse headers
	for i := 1; i < len(lines); i++ {
		line := lines[i]
		if line == "" {
			break // End of headers
		}
		
		headerParts := strings.SplitN(line, ": ", 2)
		if len(headerParts) == 2 {
			headerName := strings.ToLower(headerParts[0])
			headerValue := headerParts[1]
			
			switch headerName {
			case "host":
				req.Host = headerValue
			case "accept":
				req.Accept = headerValue
			case "accept-encoding":
				req.AcceptEncoding = headerValue
			}
		}
	}
	
	// If no accept-encoding header, set to "none"
	if req.AcceptEncoding == "" {
		req.AcceptEncoding = "none"
	}
	
	return req
}

func compressGzip(data []byte) []byte {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	writer.Write(data)
	writer.Close()
	return buf.Bytes()
}

func compressDeflate(data []byte) []byte {
	var buf bytes.Buffer
	writer, _ := flate.NewWriter(&buf, 6) // Level 6 compression as specified
	writer.Write(data)
	writer.Close()
	return buf.Bytes()
}

func ResponseEncoder(res HttpResponse) []byte {
	var responseBuilder strings.Builder
	
	// Status line
	responseBuilder.WriteString(fmt.Sprintf("%s %s OK\r\n", res.Version, res.StatusCode))
	
	// Content-Type header (if present)
	if res.ContentType != "" {
		responseBuilder.WriteString(fmt.Sprintf("Content-Type: %s\r\n", res.ContentType))
	}
	
	// Content-Encoding header (if not "none")
	if res.ContentEncoding != "" && res.ContentEncoding != "none" {
		responseBuilder.WriteString(fmt.Sprintf("Content-Encoding: %s\r\n", res.ContentEncoding))
	}
	
	// Content-Length header
	responseBuilder.WriteString(fmt.Sprintf("Content-Length: %d\r\n", res.ContentLength))
	
	// End of headers
	responseBuilder.WriteString("\r\n")
	
	// Response body
	response := []byte(responseBuilder.String())
	response = append(response, res.Data...)
	
	return response
}
