# Auth Proxy Auth

This package provides session for the other services. This involves reading from requests, validating JWTs, making sure
the database agrees with what is being passed, and then handing off to handlers with the session information in context.
