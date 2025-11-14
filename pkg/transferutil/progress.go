package transferutil

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/containerd/containerd/v2/core/transfer"
	"github.com/containerd/containerd/v2/pkg/progress"
)

// From https://github.com/containerd/containerd/blob/v2.2.0-rc.0/cmd/ctr/commands/image/pull.go#L240-L473
type progressNode struct {
	transfer.Progress
	children []*progressNode
	root     bool
}

func (n *progressNode) mainDesc() *ocispec.Descriptor {
	if n.Desc != nil {
		return n.Desc
	}
	for _, c := range n.children {
		if desc := c.mainDesc(); desc != nil {
			return desc
		}
	}
	return nil
}

// ProgressHandler returns a progress callback and a cleanup function to render transfer progress.
// This implementation is based on containerd's ctr command progress handler.
func ProgressHandler(ctx context.Context, out io.Writer) (transfer.ProgressFunc, func()) {
	ctx, cancel := context.WithCancel(ctx)
	var (
		fw       = progress.NewWriter(out)
		start    = time.Now()
		statuses = map[string]*progressNode{}
		roots    = []*progressNode{}
		pc       = make(chan transfer.Progress, 5)
		status   string
		closeC   = make(chan struct{})
	)

	progressFn := func(p transfer.Progress) {
		select {
		case pc <- p:
		case <-ctx.Done():
		}
	}

	done := func() {
		cancel()
		<-closeC
	}

	go func() {
		defer close(closeC)
		for {
			select {
			case p := <-pc:
				if p.Name == "" {
					status = p.Event
					continue
				}
				if node, ok := statuses[p.Name]; !ok {
					node = &progressNode{
						Progress: p,
						root:     true,
					}
					if len(p.Parents) == 0 {
						roots = append(roots, node)
					} else {
						var parents []string
						for _, parent := range p.Parents {
							pStatus, ok := statuses[parent]
							if ok {
								parents = append(parents, parent)
								pStatus.children = append(pStatus.children, node)
								node.root = false
							}
						}
						node.Progress.Parents = parents
						if node.root {
							roots = append(roots, node)
						}
					}
					statuses[p.Name] = node
				} else {
					if len(node.Progress.Parents) != len(p.Parents) {
						var parents []string
						var removeRoot bool
						for _, parent := range p.Parents {
							pStatus, ok := statuses[parent]
							if ok {
								parents = append(parents, parent)
								var found bool
								for _, child := range pStatus.children {
									if child.Progress.Name == p.Name {
										found = true
										break
									}
								}
								if !found {
									pStatus.children = append(pStatus.children, node)
								}
								if node.root {
									removeRoot = true
								}
								node.root = false
							}
						}
						p.Parents = parents
						// Check if needs to remove from root
						if removeRoot {
							for i := range roots {
								if roots[i] == node {
									roots = append(roots[:i], roots[i+1:]...)
									break
								}
							}
						}
					}
					node.Progress = p
				}

				displayHierarchy(fw, status, roots, start)
				fw.Flush()

			case <-ctx.Done():
				return
			}
		}
	}()

	return progressFn, done
}

func displayHierarchy(w io.Writer, status string, roots []*progressNode, start time.Time) {
	total := displayNode(w, "", roots)
	for _, r := range roots {
		if desc := r.mainDesc(); desc != nil {
			fmt.Fprintf(w, "%s %s\n", desc.MediaType, desc.Digest)
		}
	}
	// Print the Status line
	fmt.Fprintf(w, "%s\telapsed: %-4.1fs\ttotal: %7.6v\t(%v)\t\n",
		status,
		time.Since(start).Seconds(),
		progress.Bytes(total),
		progress.NewBytesPerSecond(total, time.Since(start)))
}

func displayNode(w io.Writer, prefix string, nodes []*progressNode) int64 {
	var total int64
	for i, node := range nodes {
		status := node.Progress
		total += status.Progress
		pf, cpf := prefixes(i, len(nodes))
		if node.root {
			pf, cpf = "", ""
		}

		name := prefix + pf + displayName(status.Name)

		switch status.Event {
		case "downloading", "uploading", "extracting":
			var bar progress.Bar
			if status.Total > 0.0 {
				bar = progress.Bar(float64(status.Progress) / float64(status.Total))
			}
			fmt.Fprintf(w, "%-40.40s\t%-11s\t%40r\t%8.8s/%s\t\n",
				name,
				status.Event,
				bar,
				progress.Bytes(status.Progress), progress.Bytes(status.Total))
		case "resolving", "waiting":
			bar := progress.Bar(0.0)
			fmt.Fprintf(w, "%-40.40s\t%-11s\t%40r\t\n",
				name,
				status.Event,
				bar)
		case "complete", "extracted":
			bar := progress.Bar(1.0)
			fmt.Fprintf(w, "%-40.40s\t%-11s\t%40r\t\n",
				name,
				status.Event,
				bar)
		default:
			fmt.Fprintf(w, "%-40.40s\t%s\t\n",
				name,
				status.Event)
		}
		total += displayNode(w, prefix+cpf, node.children)
	}
	return total
}

func prefixes(index, length int) (string, string) {
	if index+1 == length {
		return "└──", "   "
	}
	return "├──", "│  "
}

func displayName(name string) string {
	parts := strings.Split(name, "-")
	for i := range parts {
		parts[i] = shortenName(parts[i])
	}
	return strings.Join(parts, " ")
}

func shortenName(name string) string {
	if strings.HasPrefix(name, "sha256:") && len(name) == 71 {
		return "(" + name[7:19] + ")"
	}
	return name
}
