# sendflit — Go

Transactional email for applications and AI agents. Zero dependencies (stdlib `net/http`).

## Install

```bash
go get github.com/Sendflit/sendflit-go
```

## Usage

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/Sendflit/sendflit-go"
)

func main() {
	sfl := sendflit.New("re_...")

	res, err := sfl.Send(context.Background(), sendflit.Email{
		From:    "you@yourdomain.com",
		To:      "user@example.com",
		Subject: "Confirm your email",
		HTML:    "<p>Click <a href='...'>here</a>.</p>",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(res.ID, res.Status)
}
```

### Attachments

```go
em := sendflit.Email{From: "billing@yourdomain.com", To: "u@example.com", Subject: "Receipt"}
em.AttachBytes("receipt.pdf", pdfBytes, "application/pdf")
em.Attach("note.txt", "plain text", "")
```

### Batch (≤100), domains, webhooks

```go
sfl.Batch(ctx, []sendflit.Email{...})
sfl.AddDomain(ctx, "acme.com")
sendflit.VerifyWebhookSignature("whsec_...", rawBody, r.Header.Get("X-Sendflit-Signature"))
```

Context-aware; retries 429/5xx with `Retry-After` honored; errors are `*sendflit.APIError` with `.Status`.

MIT © SendFlit
