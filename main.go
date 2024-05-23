package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"os"

	nodeclient "github.com/celestiaorg/celestia-openrpc"
	"github.com/celestiaorg/celestia-openrpc/types/blob"
	"github.com/celestiaorg/celestia-openrpc/types/share"
	openai "github.com/sashabaranov/go-openai"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Get IP, namespace, and prompt from program arguments
	if len(os.Args) != 4 {
		log.Fatal("Usage: go run main.go <nodeIP> <namespace> <prompt>")
	}
	nodeIP, namespaceHex, prompt := os.Args[1], os.Args[2], os.Args[3]

	// We pass an empty string as the jwt token, since we
	// disabled auth with the --rpc.skip-auth flag
	client, err := nodeclient.NewClient(ctx, nodeIP, "")
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}
	defer client.Close()

	// Next, we convert the namespace hex string to the
	// concrete NamespaceID type
	namespaceID, err := createNamespaceID(namespaceHex)
	if err != nil {
		log.Fatalf("Failed to decode namespace: %v", err)
	}

	// We can then create and submit a blob using the NamespaceID and our prompt.
	createdBlob, height, err := createAndSubmitBlob(ctx, client, namespaceID, prompt)
	if err != nil {
		log.Fatal(err)
	}

	// Now we will fetch the blob back from the network.
	fetchedBlob, err := client.Blob.Get(ctx, height, namespaceID, createdBlob.Commitment)
	if err != nil {
		log.Fatalf("Failed to fetch blob: %v", err)
	}

	log.Printf("Fetched blob: %s\n", string(fetchedBlob.Data))
	promptAnswer, err := gpt3(ctx, string(fetchedBlob.Data))
	if err != nil {
		log.Fatalf("Failed to process message with GPT-3: %v", err)
	}

	log.Printf("GPT-3 response: %s\n", promptAnswer)
}

// createNamespaceID converts a hex string to a NamespaceID
func createNamespaceID(nIDString string) (share.Namespace, error) {
	// First, we parse the passed hex string into a []byte slice
	namespaceBytes, err := hex.DecodeString(nIDString)
	if err != nil {
		return nil, fmt.Errorf("error decoding hex string: %w", err)
	}

	// Next, we create a new NamespaceID using the parsed bytes
	return share.NewBlobNamespaceV0(namespaceBytes)
}

// createAndSubmitBlob creates a new blob and submits it to the network.
func createAndSubmitBlob(
	ctx context.Context,
	client *nodeclient.Client,
	ns share.Namespace,
	payload string,
) (*blob.Blob, uint64, error) {
	// First we can create the blob using the namespace and payload.
	createdBlob, err := blob.NewBlobV0(ns, []byte(payload))
	if err != nil {
		return nil, 0, fmt.Errorf("Failed to create blob: %w", err)
	}

	// After we've created the blob, we can submit it to the network.
	// Here we use the default gas price.
	height, err := client.Blob.Submit(ctx, []*blob.Blob{createdBlob}, blob.DefaultGasPrice())
	if err != nil {
		return nil, 0, fmt.Errorf("Failed to submit blob: %v", err)
	}

	log.Printf("Blob submitted successfully at height: %d! \n", height)
	log.Printf("Explorer link: https://arabica.celenium.io/block/%d \n", height)

	return createdBlob, height, nil
}

// gpt3 processes a given message using GPT-3 and returns the response.
func gpt3(ctx context.Context, msg string) (string, error) {
	// Set the authentication header
	openAIKey := os.Getenv("OPENAI_KEY")
	if openAIKey == "" {
		return "", fmt.Errorf("OPENAI_KEY environment variable not set")
	}
	client := openai.NewClient(openAIKey)
	resp, err := client.CreateChatCompletion(
		ctx,
		openai.ChatCompletionRequest{
			Model: openai.GPT3Dot5Turbo,
			Messages: []openai.ChatCompletionMessage{
				{
					Role:    openai.ChatMessageRoleUser,
					Content: msg,
				},
			},
		},
	)

	if err != nil {
		return "", fmt.Errorf("ChatCompletion error: %w", err)
	}

	return resp.Choices[0].Message.Content, nil
}
