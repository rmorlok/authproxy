# Shared Routes

This package contains routes that are shared across services. This allows the same functionality to be offered to
different parts of the app and with different security considerations (e.g. session vs no session). Individual services
will have their own service-specific routes in `services/<service>/routes`.