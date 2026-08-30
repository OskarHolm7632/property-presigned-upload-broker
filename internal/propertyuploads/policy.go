package propertyuploads

import (
	"errors"
	"fmt"
	"regexp"
)

type AssetKind string

const (
	MaintenanceRequest AssetKind = "maintenance_request"
	TenantDocument     AssetKind = "tenant_document"
	InspectionReminder AssetKind = "inspection_reminder"
)

type UploadRequest struct {
	PropertyID  string    `json:"property_id"`
	AssetID     string    `json:"asset_id"`
	Kind        AssetKind `json:"kind"`
	ContentType string    `json:"content_type"`
}

type Decision struct {
	Key            string
	ContentType    string
	MaxBytes       int64
	ExpiresSeconds int
}

var identifier = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

func DecideUpload(req UploadRequest) (Decision, error) {
	if !identifier.MatchString(req.PropertyID) || !identifier.MatchString(req.AssetID) {
		return Decision{}, errors.New("property_id and asset_id must be 1-64 URL-safe characters")
	}

	var allowed map[string]bool
	var maxBytes int64
	switch req.Kind {
	case MaintenanceRequest:
		allowed = map[string]bool{"image/jpeg": true, "image/png": true}
		maxBytes = 15 << 20
	case TenantDocument:
		allowed = map[string]bool{"application/pdf": true}
		maxBytes = 10 << 20
	case InspectionReminder:
		allowed = map[string]bool{"image/jpeg": true, "image/png": true, "application/pdf": true}
		maxBytes = 8 << 20
	default:
		return Decision{}, errors.New("unknown asset kind")
	}
	if !allowed[req.ContentType] {
		return Decision{}, fmt.Errorf("content type %q is not allowed for %s", req.ContentType, req.Kind)
	}

	return Decision{
		Key:            fmt.Sprintf("properties/%s/%s/%s", req.PropertyID, req.Kind, req.AssetID),
		ContentType:    req.ContentType,
		MaxBytes:       maxBytes,
		ExpiresSeconds: 600,
	}, nil
}
