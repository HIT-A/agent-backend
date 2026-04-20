package main

import (
	"context"
	"fmt"
	"os"

	"hoa-agent-backend/internal/cos"
)

func main() {
	fmt.Printf("COS_SECRET_ID: %s...\n", os.Getenv("COS_SECRET_ID")[:20])
	fmt.Printf("COS_SECRET_KEY: %s...\n", os.Getenv("COS_SECRET_KEY")[:20])
	fmt.Printf("COS_REGION: %s\n", os.Getenv("COS_REGION"))
	fmt.Printf("COS_BUCKET: %s\n", os.Getenv("COS_BUCKET"))

	client, err := cos.NewClientFromEnv()
	if err != nil {
		fmt.Printf("COS init error: %v\n", err)
		return
	}

	fmt.Println("COS client initialized successfully")

	storage := cos.NewStorage(client, 10*1024*1024)
	files, err := storage.ListFiles(context.Background(), "", 5)
	if err != nil {
		fmt.Printf("List files error: %v\n", err)
		return
	}

	fmt.Printf("Found %d files\n", len(files))
	for _, f := range files {
		fmt.Printf("  - %s (%d bytes)\n", f["key"], f["size"])
	}
}
