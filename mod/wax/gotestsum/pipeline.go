package gotestsum

type Pipeline struct {
	Packages []*Package
	Metadata map[string]string
}
