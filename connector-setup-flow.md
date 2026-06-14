# Connector Setup Flow

Connector setup flows let a connector collect configuration before and after credentials are established. The setup flow is authored in connector YAML and returned to clients one eligible step at a time.

## Phases

```yaml
setup_flow:
  preconnect:
    steps: []
  configure:
    steps: []
```

`preconnect` steps run before the auth method establishes credentials. Use them for values needed by the auth flow itself, such as a tenant, region, or instance URL.

`configure` steps run after credentials are available. Use them for connection-specific choices such as workspace selection, sync settings, or other post-auth options. Configure form steps can define `data_sources` because AuthProxy can call the upstream API through the authenticated proxy at that point.

Auth-method steps and AuthProxy pseudo-steps are inserted by the runtime. Connector YAML cannot conditionally hide or override them. In particular:

- Auth-method steps, such as OAuth redirect or API-key credential collection, are always eligible.
- `apxy:verify` is always eligible when verification is needed.
- Connector-authored step ids must not start with `apxy:`.

## Form Steps

Form steps are the default step type. They use JSON Schema for submitted data and optional JSONForms UI Schema for layout.

```yaml
setup_flow:
  preconnect:
    steps:
      - id: region
        title: Region
        json_schema:
          type: object
          required:
            - region
          properties:
            region:
              type: string
              enum:
                - us
                - eu
```

When a form step is submitted, AuthProxy validates the payload against the step's `json_schema` and merges only top-level fields declared in `properties` into the connection configuration.

## Redirect Steps

Redirect steps send the user to an off-platform URL before continuing setup. Set `type: redirect` and provide `redirect.url`.

```yaml
setup_flow:
  preconnect:
    steps:
      - id: external_setup
        type: redirect
        title: External setup
        redirect:
          url: https://setup.example.com/start?done={{RETURN_ADVANCE}}&cancel={{RETURN_ABORT}}
```

Redirect URLs support `{{cfg.field_name}}` mustache references plus two runtime placeholders:

- `{{RETURN_ADVANCE}}`: a signed one-time-use URL that advances the connection to the next step.
- `{{RETURN_ABORT}}`: a signed one-time-use URL that aborts the in-flight setup.

Redirect steps cannot define `json_schema`, `ui_schema`, or `data_sources`.

## Conditional Steps

Connector-authored form and redirect steps can include an `if.javascript` condition. AuthProxy evaluates the condition server-side each time it resolves the current or next setup step. Clients only receive steps whose condition is true.

```yaml
setup_flow:
  configure:
    steps:
      - id: advanced_options
        title: Advanced options
        if:
          javascript: |
            cfg.region === "eu" && labels["apxy/cxr/type"] === "salesforce"
        json_schema:
          type: object
          properties:
            sync_mode:
              type: string
```

The JavaScript must evaluate to a boolean. Syntax errors, thrown errors, `null`, `undefined`, and non-boolean results fail the setup request instead of guessing the connector author's intent.

The JavaScript runtime exposes these variables:

| Variable | Shape | Description |
|---|---|---|
| `cfg` | object | The connection configuration collected by submitted setup steps so far. It is `{}` when no configuration has been saved yet. |
| `labels` | object | The connection labels available to the runtime, including carried-forward system labels such as `apxy/cxr/type`. |
| `annotations` | object | The connection annotations available to the runtime. It is `{}` when none are present. |

Conditions are useful when a later step depends on an earlier answer, a connector label, or an operator-supplied annotation:

```yaml
setup_flow:
  preconnect:
    steps:
      - id: region
        title: Region
        json_schema:
          type: object
          required:
            - region
          properties:
            region:
              type: string
              enum:
                - us
                - eu
  configure:
    steps:
      - id: workspace
        title: Workspace
        json_schema:
          type: object
          required:
            - workspace_id
          properties:
            workspace_id:
              type: string
              x-data-source: workspaces
        data_sources:
          workspaces:
            proxy_request:
              method: GET
              url: https://api.example.com/workspaces
            transform: data.map(w => ({ value: w.id, label: w.name }))
      - id: eu_sync_options
        title: EU sync options
        if:
          javascript: |
            cfg.region === "eu" && annotations["setup-mode"] === "advanced"
        json_schema:
          type: object
          properties:
            sync_mode:
              type: string
              enum:
                - standard
                - restricted
```

## Runtime Behavior

AuthProxy applies step conditions consistently across setup APIs:

- Initiating setup starts at the first eligible step.
- Submitting a step advances to the next eligible step.
- Resuming setup advances past a stored step that has become ineligible.
- Reconfiguring a ready connection starts at the first eligible configure step.
- Data sources are available only for the current eligible configure step.

If all connector-authored steps in a phase are ineligible, the runtime skips that phase. Auth-method and verification steps still run according to the connector's auth method and probe configuration.

## Migration Notes

Adding, removing, or changing `if.javascript` changes the connector definition. For published connectors, publish a new connector version or rely on the normal connector-version migration path for configuration changes.

Existing in-flight connections are evaluated against the connector version they are using. If a connection resumes while its stored step is now ineligible, AuthProxy advances it to the next eligible step rather than returning the ineligible step to the client.
