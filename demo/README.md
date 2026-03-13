# lazygcs Demo Generator

This directory contains the scripts and assets needed to generate the `demo.gif` shown in the main `README.md`. It automatically spins up a mock GCS server so you don't need real Google Cloud credentials or data to record a demo!

## Prerequisites

1.  Ensure you have [vhs](https://github.com/charmbracelet/vhs) installed:
    ```bash
    go install github.com/charmbracelet/vhs@latest
    ```
2.  Ensure you have `ffmpeg` and `ttyd` installed (required by `vhs`).

## How to generate a new demo

1.  Run the generator script from the **root of the repository**:
    ```bash
    PATH=$PATH:$(go env GOPATH)/bin go run demo/main.go
    ```

### What happens under the hood?

When you run the script, it does the following:
1.  **Builds** the `lazygcs` binary and outputs it to `demo/lazygcs`.
2.  **Starts** an in-memory mock GCS server (using `fsouza/fake-gcs-server`) pre-populated with some fake buckets and files.
3.  **Generates** a temporary `config.toml` inside `demo/`.
4.  **Runs** `vhs demo/demo.tape` with the `LAZYGCS_CONFIG` and `STORAGE_EMULATOR_HOST` environment variables set so that the tool uses the local mock data instead of real Google Cloud data.
5.  **Saves** the resulting recording to `demo/demo.gif`.

## Customizing the recording

You can edit the terminal commands or delays by modifying `demo/demo.tape`.
To add or change the mock data, modify the `InitialObjects` slice in `demo/main.go`.