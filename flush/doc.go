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
// not possible, flush manually after each request. [EmitContext] keeps the
// upload within the request's deadline:
//
//	flush.EmitContext(ctx)
//
// A flush is only as fast as its [Storage]. To keep a stalled upload from
// blocking later flushes and shutdown, set Config.FlushTimeout, or use
// [EmitContext] and [StopContext].
//
// The [Storage] interface abstracts the destination for coverage files.
// Built-in implementations include [LocalStorage] for local directories and
// [WriterStorage] for debugging. The objstore sub-package provides
// remote storage support for S3, GCS, and Azure Blob Storage.
package flush
