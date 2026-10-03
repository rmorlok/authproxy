package main

import (
	"fmt"
	"strings"
	"time"

	sconfig "github.com/rmorlok/authproxy/internal/schema/config"
	"github.com/rmorlok/authproxy/internal/schema/resources/namespace"
)

type startupApplyOptions struct {
	createSystemActor bool
	filenames         []string
	actor             string
	actorNamespace    string
	timeout           time.Duration
}

// serveFlags holds raw CLI input. Only resolve translates it into runtime options;
// leaf operations never need to interpret comma-separated lists or the all alias.
type serveFlags struct {
	noBanner    bool
	autoMigrate bool
	apply       startupApplyOptions
}

type serveOptions struct {
	noBanner     bool
	autoMigrate  bool
	services     []sconfig.ServiceId
	apply        startupApplyOptions
	applyService sconfig.ServiceId
}

func (f serveFlags) resolve(serviceList string) (serveOptions, error) {
	o := serveOptions{
		noBanner: f.noBanner,
		autoMigrate: f.autoMigrate,
		apply: f.apply,
	}

	seen := map[sconfig.ServiceId]bool{}
	for _, value := range strings.Split(serviceList, ",") {
		ids := []sconfig.ServiceId{sconfig.ServiceId(strings.TrimSpace(value))}
		if ids[0] == "all" {
			ids = sconfig.AllServiceIds()
		}
		for _, id := range ids {
			if !sconfig.IsValidServiceId(id) {
				return o, fmt.Errorf("unknown service: %s", id)
			}
			if !seen[id] {
				o.services = append(o.services, id)
				seen[id] = true
			}
		}
	}

	if len(o.apply.filenames) == 0 {
		return o, nil
	}

	if seen[sconfig.ServiceIdAdminApi] {
		o.applyService = sconfig.ServiceIdAdminApi
	} else if seen[sconfig.ServiceIdApi] {
		o.applyService = sconfig.ServiceIdApi
	} else {
		return o, fmt.Errorf("--apply requires serving api or admin-api")
	}

	if o.apply.actor == "" {
		if o.apply.actorNamespace != namespace.Root {
			return o, fmt.Errorf("--apply-actor-namespace requires --apply-actor; the default system actor is in root")
		}

		o.apply.actor = "system"
		o.apply.createSystemActor = true
	}

	if err := namespace.ValidatePath(o.apply.actorNamespace); err != nil {
		return o, fmt.Errorf("--apply-actor-namespace: %w", err)
	}

	if o.apply.timeout <= 0 {
		return o, fmt.Errorf("--apply-timeout must be positive")
	}

	return o, nil
}
