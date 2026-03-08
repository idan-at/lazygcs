# Releasing lazygcs

This project uses [GoReleaser](https://goreleaser.com/) and GitHub Actions to automate the release process. 

## Automated Release (Tag-Driven)

To publish a new version, simply push a Git tag. GitHub Actions will automatically cross-compile the binaries, generate checksums, and create a GitHub Release with the appropriate assets.

### Steps:

1.  **Ensure you are on the `main` branch and it is up to date:**
    ```bash
    git checkout main
    git pull origin main
    ```

2.  **Create a new version tag:**
    Follow [Semantic Versioning](https://semver.org/) (e.g., `v0.1.0`, `v1.0.0`).
    ```bash
    git tag -a v0.1.0 -m "Release v0.1.0"
    ```

3.  **Push the tag to GitHub:**
    ```bash
    git push origin v0.1.0
    ```

4.  **Monitor the Release:**
    Go to the **Actions** tab on your GitHub repository. You will see a "Release" workflow running. Once finished, the new version will appear under the **Releases** section.

## Local Testing (Dry Run)

If you have `goreleaser` installed locally and want to verify the build process without actually publishing anything:

```bash
goreleaser release --snapshot --clean --skip=publish
```

This will build the binaries into the `dist/` directory but won't create a GitHub release.

## How Versioning Works

The version displayed by `lazygcs --version` is dynamically injected at build time.
*   In `main.go`, `var version = "dev"` is the default.
*   GoReleaser uses `-ldflags="-X main.version={{.Version}}"` to overwrite this variable with the current Git tag during the build process.
