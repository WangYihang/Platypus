package models

// Environment represents the environment of the application
type Environment string

const (
	// Development represents the development environment
	Development Environment = "development"
	// Staging represents the staging environment
	Staging Environment = "staging"
	// Production represents the production environment
	Production Environment = "production"
)
