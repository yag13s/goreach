// Package flush provides zero-dependency runtime coverage collection for Go services.
//
// It captures coverage data from running processes built with -cover flag,
// without requiring the process to stop.
//
// Basic usage:
//
//	flush.Enable(flush.Config{
//	    ServiceName:  "my-service",
//	    BuildVersion: "abc1234",
//	    Interval:     30 * time.Second,
//	    Clear:        true,
//	})
//	defer flush.Stop()
//
// For serverless environments (e.g., AWS Lambda) where periodic flushing is
// not possible, call [Emit] manually after each request:
//
//	flush.Emit()
//
// The [Storage] interface abstracts the destination for coverage files.
// Built-in implementations include [LocalStorage] for local directories and
// [WriterStorage] for debugging. The objstore sub-package provides
// remote storage support for S3, GCS, and Azure Blob Storage.
package flush
