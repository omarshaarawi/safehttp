# Safe HTTP Client

This project provides a Go implementation of a safe HTTP client with enhanced safety features like circuit breaking.

## Features

- Circuit Breaking: The client can stop sending requests based on the error rate and retry rate within a certain time
  window.
- Retry Logic: The client automatically retries failed requests with a backoff time calculation.
- Error Tracking: The client keeps track of the total requests, failed requests, and retries within a time window.

## Getting Started

### Prerequisites

- Go 1.21.2 or higher

### Installation

To start using this project, run:

```bash
go get github.com/omarshaarawi/safehttp
```

## Usage

Here is a basic example of how to use the SafeClient:

```go
package main

import (
	"github.com/omarshaarawi/safehttp"
	"net/http"
	"time"
)

func main() {
	client := http.Client{}
	safeClient := safehttp.New(&client, 0.6, 0.4, time.Minute)

	req, _ := http.NewRequest("GET", "http://example.com", nil)
	resp, err := safeClient.SendRequest(req)
	if err != nil {
		log.Fatal(err)
	}
	// handle resp
}
```

## Configuration

The `SafeClient` struct in the `safe_http.go` file has the following configuration fields:

| Configuration  | Type          | Default Value | Description                                                  |
|----------------|---------------|---------------|--------------------------------------------------------------|
| ErrorThreshold | float64       | None          | The error rate threshold for triggering the circuit breaker. |
| RetryThreshold | float64       | None          | The retry rate threshold for triggering the circuit breaker. |
| WindowDuration | time.Duration | None          | The time window for calculating the error and retry rates.   |

## Contributing

Contributions are welcome. Please open an issue or submit a pull request.
