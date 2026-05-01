package provider

import (
	"fmt"
	"sync"
)

type ProviderFactory func(opts ...Option) (LLMProvider, error)

var (
	registryMu       sync.RWMutex
	providerRegistry = map[string]ProviderFactory{}
)

func RegisterProvider(name string, factory ProviderFactory) error {
	if name == "" {
		return fmt.Errorf("%w: empty name", ErrInvalidInput)
	}
	if factory == nil {
		return fmt.Errorf("%w: nil factory", ErrInvalidInput)
	}

	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := providerRegistry[name]; exists {
		return fmt.Errorf("%w: %s", ErrProviderAlreadyRegistered, name)
	}
	providerRegistry[name] = factory
	return nil
}

func MustRegisterProvider(name string, factory ProviderFactory) {
	if err := RegisterProvider(name, factory); err != nil {
		panic(err)
	}
}

// lookupProvider fetches the factory for a provider name.
func lookupProvider(name string) (ProviderFactory, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	factory, ok := providerRegistry[name]
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrProviderNotFound, name)
	}
	return factory, nil
}

func New(providerName string, opts ...Option) (LLMProvider, error) {
	factory, err := lookupProvider(providerName)
	if err != nil {
		return nil, err
	}
	provider, err := factory(opts...)
	if err != nil {
		return nil, err
	}

	return provider, nil
}

// MustNew is a helper that panics if client creation fails.
func MustNew(providerName string, opts ...Option) LLMProvider {
	client, err := New(providerName, opts...)
	if err != nil {
		panic(err)
	}
	return client
}

func RegisteredProviders() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(providerRegistry))
	for name := range providerRegistry {
		names = append(names, name)
	}
	return names
}
