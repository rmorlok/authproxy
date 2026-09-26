package apply

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sync"

	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

// Kustomize uses global OpenAPI state; serialize builds within this process.
var kustomizeMu sync.Mutex

func LoadKustomize(ctx context.Context, directory string, options Options) ([]Document, error) {
	if len(options.Filenames) != 0 || options.Recursive {
		return nil, fmt.Errorf("--kustomize is mutually exclusive with --filename and --recursive")
	}
	info, err := os.Stat(directory)
	if err != nil || !info.IsDir() {
		return nil, fmt.Errorf("--kustomize requires a local directory")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	kustomizeMu.Lock()
	defer kustomizeMu.Unlock()
	resources, err := krusty.MakeKustomizer(krusty.MakeDefaultOptions()).Run(filesys.MakeFsOnDisk(), directory)
	if err != nil {
		return nil, fmt.Errorf("Kustomize build failed (check kustomization inputs)")
	}
	data, err := resources.AsYaml()
	if err != nil {
		return nil, fmt.Errorf("cannot encode Kustomize output")
	}
	options.Filenames = []string{"-"}
	options.Stdin = bytes.NewReader(data)
	docs, err := Load(ctx, options)
	if err != nil {
		return nil, fmt.Errorf("Kustomize output: %w", err)
	}
	for i := range docs {
		docs[i].Source = "kustomize: " + docs[i].Source
	}
	return docs, nil
}
