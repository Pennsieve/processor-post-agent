package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
)

var CommandRunDirectory = "/service"

func main() {
	fmt.Println("Welcome to the Post-Processor")
	integrationID := os.Getenv("INTEGRATION_ID")
	environment := os.Getenv("ENVIRONMENT")

	// get integration
	sessionToken := os.Getenv("SESSION_TOKEN")
	apiHost2 := os.Getenv("PENNSIEVE_API_HOST2")
	integrationResponse, err := getIntegration(apiHost2, integrationID, sessionToken)
	if err != nil {
		log.Fatalln(err)
	}
	fmt.Println(string(integrationResponse))
	var integration Integration
	if err := json.Unmarshal(integrationResponse, &integration); err != nil {
		fmt.Println(err.Error())
	}
	fmt.Println(integration)

	datasetID := integration.DatasetNodeID

	var target_path string
	if integration.Params != nil {
		params := integration.Params.(map[string]interface{})

		target_path_val, ok := params["target_path"]
		if ok {
			target_path = fmt.Sprintf("%v", target_path_val)
		}
	}
	fmt.Println("target path", target_path)
	setErr := os.Setenv("TARGET_PATH", target_path)
	if setErr != nil {
		fmt.Println("error setting variable TARGET_PATH:",
			setErr)
	}

	fmt.Println("ENVIRONMENT: ", environment)
	fmt.Println("PENNSIEVE_API_HOST: ", os.Getenv("PENNSIEVE_API_HOST"))
	fmt.Println("PENNSIEVE_UPLOAD_BUCKET: ", os.Getenv("PENNSIEVE_UPLOAD_BUCKET"))
	if environment == "prod" {
		fmt.Println("unsetting variables")
		apiHostErr := os.Unsetenv("PENNSIEVE_API_HOST")
		if apiHostErr != nil {
			fmt.Println("error unsetting variable PENNSIEVE_API_HOST:",
				apiHostErr)
		}
		err := os.Unsetenv("PENNSIEVE_UPLOAD_BUCKET")
		if err != nil {
			fmt.Println("error unsetting variable PENNSIEVE_UPLOAD_BUCKET:",
				err)
		}
	}
	fmt.Println("PENNSIEVE_API_HOST: ", os.Getenv("PENNSIEVE_API_HOST"))
	fmt.Println("PENNSIEVE_UPLOAD_BUCKET: ", os.Getenv("PENNSIEVE_UPLOAD_BUCKET"))
	fmt.Println("DATASET_ID: ", datasetID)
	fmt.Println("INTEGRATION_ID: ", integrationID)

	// validate
	inputDir := os.Getenv("INPUT_DIR")
	outputDir := os.Getenv("OUTPUT_DIR")

	var processorInputDir string
	processorInputDir = outputDir
	if integration.WorkflowUuid != "" {
		fmt.Println("workflowUuid detected, using INPUT_DIR for processor input")
		processorInputDir = inputDir
	}

	entries, err := os.ReadDir(processorInputDir)

	if err != nil {
		log.Fatal(err)
	}

	if len(entries) == 0 {
		fmt.Println("error, no files to process")
		os.Exit(1)
	}

	// Ensure API credentials are available
	createdAPIKey, apiHost, err := ensureAPICredentials(environment, sessionToken, integrationID)
	if err != nil {
		fmt.Println("failed to ensure API credentials", slog.String("error", err.Error()))
		os.Exit(1)
	}

	cmd := exec.Command("/bin/sh", "./agent.sh", datasetID, integrationID, processorInputDir)
	out, err := cmd.Output()
	if err != nil {
		log.Fatalf("error %s", err)
	}
	output := string(out)
	fmt.Println(output)

	// Clean up created API key if one was created
	if createdAPIKey != "" {
		if err := deleteAPIKey(apiHost, sessionToken, createdAPIKey); err != nil {
			fmt.Println("failed to delete API key", slog.String("error", err.Error()))
		}
	}
}

type Integration struct {
	Uuid          string      `json:"uuid"`
	ApplicationID int64       `json:"applicationId"`
	DatasetNodeID string      `json:"datasetId"`
	PackageIDs    []string    `json:"packageIds"`
	Params        interface{} `json:"params"`
	WorkflowUuid  string      `json:"workflowUuid"`
}

func getIntegration(apiHost string, integrationId string, sessionToken string) ([]byte, error) {
	url := fmt.Sprintf("%s/compute/workflows/instances/%s", apiHost, integrationId)

	req, _ := http.NewRequest("GET", url, nil)

	req.Header.Add("accept", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", sessionToken))

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}

	defer res.Body.Close()
	body, _ := io.ReadAll(res.Body)

	return body, nil
}

type APIKeyResponse struct {
	Name   string `json:"name"`
	Key    string `json:"key"`
	Secret string `json:"secret"`
}

func createAPIKey(apiHost string, sessionToken string, name string) (*APIKeyResponse, error) {
	url := fmt.Sprintf("%s/token?api_key", apiHost)

	requestBody := map[string]string{"name": name}
	jsonBody, _ := json.Marshal(requestBody)

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", sessionToken))

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create API key with status: %d", res.StatusCode)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var apiKeyResponse APIKeyResponse
	if err := json.Unmarshal(body, &apiKeyResponse); err != nil {
		return nil, fmt.Errorf("failed to decode API key response: %w", err)
	}

	return &apiKeyResponse, nil
}

func ensureAPICredentials(environment string, sessionToken string, integrationID string) (string, string, error) {
	apiKey := os.Getenv("PENNSIEVE_API_KEY")
	apiSecret := os.Getenv("PENNSIEVE_API_SECRET")

	// Determine API host for token endpoint
	var apiHost string
	if environment == "local" || environment == "dev" {
		apiHost = "https://api.pennsieve.net"
	} else {
		apiHost = "https://api.pennsieve.io"
	}

	if apiKey != "" && apiSecret != "" {
		fmt.Println("PENNSIEVE_API_KEY and PENNSIEVE_API_SECRET already set")
		return "", apiHost, nil
	}

	fmt.Println("PENNSIEVE_API_KEY or PENNSIEVE_API_SECRET not set, creating API key")

	// Create API key
	apiKeyResponse, err := createAPIKey(apiHost, sessionToken, fmt.Sprintf("workflow-%s", integrationID))
	if err != nil {
		return "", apiHost, fmt.Errorf("failed to create API key: %w", err)
	}
	fmt.Println("created API key for workflow", slog.String("integrationID", integrationID))

	// Set environment variables from response
	if err := os.Setenv("PENNSIEVE_API_KEY", apiKeyResponse.Key); err != nil {
		return "", apiHost, fmt.Errorf("failed to set PENNSIEVE_API_KEY: %w", err)
	}
	if err := os.Setenv("PENNSIEVE_API_SECRET", apiKeyResponse.Secret); err != nil {
		return "", apiHost, fmt.Errorf("failed to set PENNSIEVE_API_SECRET: %w", err)
	}

	fmt.Println("PENNSIEVE_API_KEY and PENNSIEVE_API_SECRET set from API key response")
	return apiKeyResponse.Key, apiHost, nil
}

func deleteAPIKey(apiHost string, sessionToken string, apiKey string) error {
	url := fmt.Sprintf("%s/token/%s?api_key", apiHost, apiKey)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("failed to create delete request: %w", err)
	}

	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", sessionToken))

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete API key: %w", err)
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK && res.StatusCode != http.StatusNoContent {
		return fmt.Errorf("failed to delete API key with status: %d", res.StatusCode)
	}

	fmt.Println("successfully deleted API key", slog.String("apiKey", apiKey))
	return nil
}
