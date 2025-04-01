package utils

import (
	"errors"
)

// Error definitions
var (
	ErrHomeNotFound      = errors.New("home directory not found")
	ErrNoCurrentContext  = errors.New("no current kubernetes context found")
	ErrNamespaceNotFound = errors.New("namespace not found")
	ErrPodNotFound       = errors.New("pod not found")
	ErrNodeNotFound      = errors.New("node not found")
	ErrClusterInfoRead   = errors.New("failed to read cluster info")
	ErrClusterInfoWrite  = errors.New("failed to write cluster info")
)
