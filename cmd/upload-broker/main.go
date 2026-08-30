package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"time"

	"example.com/property-upload-broker/internal/propertyuploads"
)

type broker struct {
	storage *propertyuploads.InfraiClient
	bucket  string
}

type uploadIntent struct {
	UploadURL      string            `json:"upload_url"`
	Method         string            `json:"method"`
	Fields         map[string]string `json:"fields,omitempty"`
	ObjectKey      string            `json:"object_key"`
	ExpiresSeconds int               `json:"expires_seconds"`
	MaxBytes       int64             `json:"max_bytes"`
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime | log.LUTC)
	apiKey := os.Getenv("INFRAI_API_KEY")
	if apiKey == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	bucket := envOr("UPLOAD_BUCKET", "property-assets")
	client := &propertyuploads.InfraiClient{APIKey: apiKey}

	if len(os.Args) == 2 && os.Args[1] == "setup" {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := client.CreateBucket(ctx, bucket); err != nil {
			log.Fatal(err)
		}
		log.Printf("bucket %q is ready", bucket)
		return
	}
	if len(os.Args) != 1 && !(len(os.Args) == 2 && os.Args[1] == "serve") {
		log.Fatal("usage: upload-broker [setup|serve]")
	}

	b := &broker{storage: client, bucket: bucket}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /upload-intents", b.createUploadIntent)
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	addr := envOr("LISTEN_ADDR", ":8080")
	log.Printf("upload broker listening on %s bucket=%q", addr, bucket)
	log.Fatal(http.ListenAndServe(addr, mux))
}

func (b *broker) createUploadIntent(w http.ResponseWriter, r *http.Request) {
	var input propertyuploads.UploadRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&input); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON request"})
		return
	}
	decision, err := propertyuploads.DecideUpload(input)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]string{"error": err.Error()})
		return
	}
	signed, err := b.storage.PresignPut(r.Context(), b.bucket, decision, "upload:"+decision.Key)
	if err != nil {
		log.Printf("presign rejected kind=%s property=%s: %v", input.Kind, input.PropertyID, err)
		writeJSON(w, propertyuploads.ClientStatus(err), map[string]string{"error": err.Error()})
		return
	}
	method := signed.Method
	if method == "" {
		method = http.MethodPut
	}
	writeJSON(w, http.StatusCreated, uploadIntent{
		UploadURL:      signed.URL,
		Method:         method,
		Fields:         signed.Fields,
		ObjectKey:      decision.Key,
		ExpiresSeconds: decision.ExpiresSeconds,
		MaxBytes:       decision.MaxBytes,
	})
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(value); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("write response: %v", err)
	}
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
