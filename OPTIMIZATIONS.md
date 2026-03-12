# lazygcs Optimization Plan

This document outlines the strategy for making `lazygcs` significantly faster and more responsive.

## 1. Concurrent Project Loading (Status: Completed)
Parallelize the fetching of buckets across multiple Google Cloud projects during startup.
*   Modify `internal/gcs/client.go:ListBuckets` to use goroutines (e.g., `errgroup`) instead of sequential iteration.
*   **Benefit:** Drastically reduces initial startup time (`Init()`) when multiple projects are configured.

## 2. Debouncing Preview Requests (Status: Completed)
Prevent API spam when scrolling rapidly through the list.
*   Introduce a delay before triggering `fetchContent` or `fetchPrefixMetadataByName` when the cursor moves.
*   Cancel previous fetches if the cursor moves again within the window.
*   **Benefit:** Reduces unnecessary API calls and CPU overhead, preventing UI lag.

## 3. In-Memory Caching (Status: Completed)
Enable zero-latency navigation for previously visited folders/buckets.
*   Implement a cache for `ListObjects` results (keyed by bucket + prefix).
*   Cache `GetObjectContent` and `GetObjectMetadata` results.
*   Cache entries use a 5-minute TTL to ensure data stays relatively fresh without requiring manual invalidation.
*   **Benefit:** Instant rendering when navigating back (`h`) and forward (`l`) to known paths.

## 4. Predictive Prefetching (Status: Completed)
Anticipate user actions to hide latency.
*   When hovering on a directory/bucket for a set time, kick off a background `ListObjects` fetch.
*   Store the result silently in the cache.
*   **Benefit:** Near-instant rendering when the user finally enters the directory.

## 5. Progressive Loading for Large Buckets (Status: Completed)
Prevent the TUI from blocking when entering a folder with 10,000+ objects.
*   Modify `fetchObjects` to yield messages in chunks (e.g., `ObjectsPageMsg`).
*   Render the first page immediately while background fetches continue appending to the list.
*   **Benefit:** Immediate feedback even for massive directories.
