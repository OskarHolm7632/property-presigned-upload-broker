package propertyuploads

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPresignPutRetainsPostFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"ok":true,"data":{"url":"https://uploads.example","method":"POST","fields":{"key":"objects/photo.jpg","policy":"signed-policy"}}}`)
	}))
	defer server.Close()

	client := &InfraiClient{APIKey: "test", BaseURL: server.URL}
	got, err := client.PresignPut(context.Background(), "assets", Decision{
		Key: "objects/photo.jpg", ExpiresSeconds: 600, ContentType: "image/jpeg", MaxBytes: 1024,
	}, "upload:photo")
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != http.MethodPost || got.Fields["key"] != "objects/photo.jpg" || got.Fields["policy"] != "signed-policy" {
		t.Fatalf("PresignPut() = %#v, want POST fields retained", got)
	}
}
