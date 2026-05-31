package kafka

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Registry is a minimal Schema Registry REST client: enough to fetch the
// latest schema id for a subject and load it into an AvroSerde at startup.
type Registry struct {
	baseURL string
	http    *http.Client
}

func NewRegistry(baseURL string) *Registry {
	return &Registry{baseURL: baseURL, http: &http.Client{Timeout: 10 * time.Second}}
}

type subjectVersion struct {
	ID     int    `json:"id"`
	Schema string `json:"schema"`
}

// LatestID returns the id and schema text of a subject's latest version.
func (r *Registry) LatestID(subject string) (int, string, error) {
	url := fmt.Sprintf("%s/subjects/%s/versions/latest", r.baseURL, subject)
	resp, err := r.http.Get(url)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return 0, "", fmt.Errorf("registry %s: %s", subject, string(body))
	}
	var sv subjectVersion
	if err := json.Unmarshal(body, &sv); err != nil {
		return 0, "", err
	}
	return sv.ID, sv.Schema, nil
}

// Register posts a schema to a subject and returns its id.
func (r *Registry) Register(subject, schema string) (int, error) {
	url := fmt.Sprintf("%s/subjects/%s/versions", r.baseURL, subject)
	payload, _ := json.Marshal(map[string]string{"schema": schema, "schemaType": "AVRO"})
	resp, err := r.http.Post(url, "application/vnd.schemaregistry.v1+json", bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("register %s: %s", subject, string(body))
	}
	var out struct {
		ID int `json:"id"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return 0, err
	}
	return out.ID, nil
}

// LoadSchemas resolves each subject's latest schema, registers it into the
// serde, and returns a name->id map so producers can pick the right id.
func (r *Registry) LoadSchemas(serde *AvroSerde, subjects map[string]string) (map[string]int, error) {
	ids := make(map[string]int, len(subjects))
	for name, subject := range subjects {
		id, schema, err := r.LatestID(subject)
		if err != nil {
			return nil, err
		}
		if err := serde.Register(id, schema); err != nil {
			return nil, err
		}
		ids[name] = id
	}
	return ids, nil
}
