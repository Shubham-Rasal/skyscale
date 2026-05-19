package vm

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

// Environment variable names
const (
	EnvVMKernelPath = "FAAS_VM_KERNEL_PATH"
	EnvVMRootFSPath = "FAAS_VM_ROOTFS_PATH"
	EnvVMMemoryMB   = "FAAS_VM_MEMORY_MB"
	EnvVMCPUCount   = "FAAS_VM_CPU_COUNT"
)

const (
	defaultVMDir      = "/opt/skyscale/vm"
	kernelFilename    = "vmlinux-5.10.225"
	rootfsFilename    = "rootfs.ext4"
	kernelDownloadURL = "https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/x86_64/kernels/vmlinux.bin"
	rootfsDownloadURL = "https://s3.amazonaws.com/spec.ccfc.min/img/quickstart_guide/x86_64/rootfs/bionic.rootfs.ext4"
)

// getDefaultKernelPath returns the kernel path, downloading it if needed.
func getDefaultKernelPath() string {
	if path := os.Getenv(EnvVMKernelPath); path != "" {
		return path
	}
	path := filepath.Join(defaultVMDir, kernelFilename)
	if err := ensureFile(path, kernelDownloadURL); err != nil {
		fmt.Fprintf(os.Stderr, "vm: kernel download failed: %v\n", err)
	}
	return path
}

// getDefaultRootFSPath returns the rootfs path, downloading it if needed.
func getDefaultRootFSPath() string {
	if path := os.Getenv(EnvVMRootFSPath); path != "" {
		return path
	}
	path := filepath.Join(defaultVMDir, rootfsFilename)
	if err := ensureFile(path, rootfsDownloadURL); err != nil {
		fmt.Fprintf(os.Stderr, "vm: rootfs download failed: %v\n", err)
	}
	return path
}

// ensureFile downloads url to dest if dest doesn't already exist.
func ensureFile(dest, url string) error {
	if _, err := os.Stat(dest); err == nil {
		return nil // already present
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "vm: downloading %s → %s\n", url, dest)
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, url)
	}
	tmp := dest + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		os.Remove(tmp)
		return err
	}
	f.Close()
	return os.Rename(tmp, dest)
}

// getDefaultMemoryMB returns the default memory in MB
func getDefaultMemoryMB() int {
	// Check environment variable first
	if mem := os.Getenv(EnvVMMemoryMB); mem != "" {
		if val, err := strconv.Atoi(mem); err == nil && val > 0 {
			return val
		}
	}
	// Default to 128MB
	return 128
}

// getDefaultCPUCount returns the default CPU count
func getDefaultCPUCount() int {
	// Check environment variable first
	if cpu := os.Getenv(EnvVMCPUCount); cpu != "" {
		if val, err := strconv.Atoi(cpu); err == nil && val > 0 {
			return val
		}
	}
	// Default to 1 CPU
	return 1
}
