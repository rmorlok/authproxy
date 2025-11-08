# Auth Methods

This packet is all the implementation of auth methods within AuthProxy. An auth method will define
how to apply authentication to a request to implement the proxy related methods. Depending on the type
it may directly manage state in the database to track things like tokens.