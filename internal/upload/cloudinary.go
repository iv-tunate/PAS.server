package upload

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"
)

type CloudinarySigner struct {
	cloudName string
	apiKey    string
	apiSecret string
}

func NewCloudinarySigner(cloudName, apiKey, apiSecret string) *CloudinarySigner {
	return &CloudinarySigner{cloudName: cloudName, apiKey: apiKey, apiSecret: apiSecret}
}

type SignedUpload struct {
	CloudName string `json:"cloud_name"`
	APIKey    string `json:"api_key"`
	Timestamp int64  `json:"timestamp"`
	Signature string `json:"signature"`
	Folder    string `json:"folder"`
}

// Sign produces a fresh, single-use signature for uploading into the
// given folder (e.g. "courses", "pastor-photo"). Cloudinary's signing
// algorithm: sort all params (except file/api_key/signature) alphabetically,
// join as key=value pairs with &, append the API secret, then SHA-1 it.
func (s *CloudinarySigner) Sign(folder string) SignedUpload {
	timestamp := time.Now().Unix()

	params := map[string]string{
		"timestamp": fmt.Sprintf("%d", timestamp),
		"folder":    folder,
	}

	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, fmt.Sprintf("%s=%s", k, params[k]))
	}
	toSign := strings.Join(pairs, "&") + s.apiSecret

	sum := sha1.Sum([]byte(toSign))

	return SignedUpload{
		CloudName: s.cloudName,
		APIKey:    s.apiKey,
		Timestamp: timestamp,
		Signature: hex.EncodeToString(sum[:]),
		Folder:    folder,
	}
}
