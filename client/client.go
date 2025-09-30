package main

import (
	"bufio"
	"encoding/json"
	"fmt"
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

	// Get URL from user
	fmt.Print("Input URL: ")
	inputURL, _ := reader.ReadString('\n')
	inputURL = strings.TrimSpace(inputURL)

	// Parse URL to extract components
	parsedURL, err := url.Parse(inputURL)
	if err != nil {
		fmt.Printf("Error parsing URL: %v\n", err)
		return
	}

	// Extract host and port
	host := parsedURL.Hostname()
	port := parsedURL.Port()
	if port == "" {
		port = "80" // Default HTTP port
	}
	uri := parsedURL.Path
	if uri == "" {
		uri = "/"
	}

	// Get Content Type from user
	fmt.Print("Input Content Type: ")
	contentType, _ := reader.ReadString('\n')
	contentType = strings.TrimSpace(contentType)

	// Get Accept Encoding from user
	fmt.Print("Input Accept Encoding (write \"none\" if no special encoding can be accepted): ")
	acceptEncoding, _ := reader.ReadString('\n')
	acceptEncoding = strings.TrimSpace(acceptEncoding)

	// Validate accept encoding
	if acceptEncoding != "none" && acceptEncoding != "gzip" && acceptEncoding != "deflate" {
		fmt.Printf("Invalid encoding: %s. Only 'gzip', 'deflate', or 'none' are allowed.\n", acceptEncoding)
		return
	}

	// Create HTTP request struct
	httpReq := HttpRequest{
		Method:         "GET",
		Uri:            uri,
		Version:        "HTTP/1.1",
		Host:           host + ":" + port,
		Accept:         contentType,
		AcceptEncoding: acceptEncoding,
	}

	// Connect to server
	serverAddr := host + ":" + port
	connection, err := net.Dial(SERVER_TYPE, serverAddr)
	if err != nil {
		fmt.Printf("Error connecting to server: %v\n", err)
		return
	}
	defer connection.Close()

	// Send request and get response
	response := Fetch(httpReq, connection)

	// Display response
	fmt.Printf("Status Code: %s\n", response.StatusCode)
	if response.ContentEncoding != "" && response.ContentEncoding != "none" {
		fmt.Printf("Encoded: %s\n", response.ContentEncoding)
	}
	
	bodyStr := strings.TrimSpace(string(response.Data))
	fmt.Printf("Body: %s\n", bodyStr)

	// If response is JSON, parse and display parsed version
	if strings.Contains(response.ContentType, "application/json") && len(response.Data) > 0 {
		var greetResponse GreetResponse
		err := json.Unmarshal(response.Data, &greetResponse)
		if err == nil {
			fmt.Printf("Parsed: %v\n", greetResponse)
		}
	}
}

func Fetch(req HttpRequest, connection net.Conn) HttpResponse {
	// Encode the request
	requestBytes := RequestEncoder(req)
	
	// Send the request to the server
	_, err := connection.Write(requestBytes)
	if err != nil {
		fmt.Printf("Error sending request: %v\n", err)
		return HttpResponse{}
	}
	
	// Read the response from the server
	buffer := make([]byte, BUFFER_SIZE)
	var responseData []byte
	
	for {
		n, err := connection.Read(buffer)
		if err != nil {
			if n == 0 {
				break // Connection closed
			}
			fmt.Printf("Error reading response: %v\n", err)
			break
		}
		responseData = append(responseData, buffer[:n]...)
		
		// Check if we have received the complete response
		responseStr := string(responseData)
		if strings.Contains(responseStr, "\r\n\r\n") {
			// We have headers, now check if we have the body
			headerEndIndex := strings.Index(responseStr, "\r\n\r\n")
			headers := responseStr[:headerEndIndex]
			
			// Look for Content-Length header
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
			break // Received less than buffer size, likely end of response
		}
	}
	
	// Decode the response
	return ResponseDecoder(responseData)
}

func ResponseDecoder(bytestream []byte) HttpResponse {
	responseStr := string(bytestream)
	lines := strings.Split(responseStr, "\r\n")
	
	response := HttpResponse{}
	
	// Parse status line
	if len(lines) > 0 {
		statusParts := strings.Split(lines[0], " ")
		if len(statusParts) >= 3 {
			response.Version = statusParts[0]
			response.StatusCode = statusParts[1]
		}
	}
	
	// Parse headers
	headerEndIndex := 0
	for i, line := range lines {
		if line == "" {
			headerEndIndex = i
			break
		}
		
		if i == 0 {
			continue // Skip status line
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
	
	// Parse body
	if headerEndIndex < len(lines)-1 {
		bodyLines := lines[headerEndIndex+1:]
		body := strings.Join(bodyLines, "\r\n")
		response.Data = []byte(body)
	}
	
	return response
}

func RequestEncoder(req HttpRequest) []byte {
	var requestBuilder strings.Builder
	
	// Request line: METHOD URI VERSION
	requestBuilder.WriteString(fmt.Sprintf("%s %s %s\r\n", req.Method, req.Uri, req.Version))
	
	// Host header
	requestBuilder.WriteString(fmt.Sprintf("Host: %s\r\n", req.Host))
	
	// Accept header
	requestBuilder.WriteString(fmt.Sprintf("Accept: %s\r\n", req.Accept))
	
	// Accept-Encoding header (only if not "none")
	if req.AcceptEncoding != "none" {
		requestBuilder.WriteString(fmt.Sprintf("Accept-Encoding: %s\r\n", req.AcceptEncoding))
	}
	
	// End of headers
	requestBuilder.WriteString("\r\n")
	
	return []byte(requestBuilder.String())
}
