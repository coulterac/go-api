# auth

Auth is a library to handle all things auth. Currently, it only handles service to service
authentication. It provides a way to request a token to access other services as well as verify that
incoming requests have permission to access a protected resource. For service to service auth we are
using OAuth 2.0 client credential grant flow.

See https://www.oauth.com/oauth2-servers/access-tokens/client-credentials/ for more details on how
things work behind the scenes.

## Getting a token to call Service B from Service A

### Manually
```go
granter := &auth.Granter{
    ClientID:     "ee58c91d-de3b-4bf1-917f-9f4c47c9de50", // A's client ID
    ClientSecret: "94bcf5c3-c911-4ede-bee3-256902540806", // A's client secret
    TenantURL: "https://redventures.auth0.com", // TenantID for environment (prod vs non-prod)
}

// request token for B
jwt, err := granter.GetToken("https://cyberdyne-robot.com") // B's Resource URI

// set the header
req, err := http.NewRequest("GET", endpointForServiceB, nil)
if err != nil {
    return
}

req.Header.Add("Authorization", "Bearer "+jwt)

client.Do(req)
```

### Using the NewRequest helper
```go
granter := &auth.Granter{
    ClientID:     "ee58c91d-de3b-4bf1-917f-9f4c47c9de50", // A's client ID
    ClientSecret: "94bcf5c3-c911-4ede-bee3-256902540806", // A's client secret
    TenantURL: "https://redventures.auth0.com", // TenantID for environment (prod vs non-prod)
}

// make a new request function for B
newRequest := granter.NewRequestFunc("https://cyberdyne-robot.com")

// Build a new request for B. The authorization header will get set behind the scenes, and a token
// will be requested if a valid one does not currently exist in the cache.
req, err := newRequest("GET", endpointForServiceB, nil)

// make the request
client.Do(req)
```


### Using the NewRoundTripper helper
```go
granter := &auth.Granter{
    ClientID:     "ee58c91d-de3b-4bf1-917f-9f4c47c9de50", // A's client ID
    ClientSecret: "94bcf5c3-c911-4ede-bee3-256902540806", // A's client secret
    TenantURL: "https://redventures.auth0.com", // TenantID for environment (prod vs non-prod)
}

// make a new http client that automatically provides auth for B on each request
client := &http.Client{}
client.Transport = auth.NewRoundTripper(granter, "https://cyberdyne-robot.com", client.Transport)


// make the request. The authorization header will get set behind the scenes, and a token will be requested if a valid 
// one does not currently exist in the cache.
req, err := http.NewRequest("GET", endpointForServiceB, nil)
if err != nil {
    return
}
client.Do(req)
```

## Authenticating an Incoming Request to Service B from Service A

### Manually
```go
// setup verifier
verifier := &auth.Verifier{
    Resource: "https://cyberdyne-robot.com", // B's Resource URI
    TenantURL: "https://redventures.auth0.com", // TenantID for environment (prod vs non-prod)
}

// parse and verify the token
parsed, err := verifier.VerifyToken(someJWTString)
```

### With Middleware
```go
// setup verifier to use in middleware
verifier := &auth.Verifier{
    Resource: "https://cyberdyne-robot.com", // B's Resource URI
    TenantURL: "https://redventures.auth0.com", // TenantURL for environment (prod vs non-prod)
}

// setup middleware
checkJWT := middleware.CheckJWT(verifier)

// register middleware
http.ListenAndServe(":3000", checkJWT(mux))
```

## Authorizing requests

You'll need to authenticate a request in order to authorize it. Authorization middleware must run
after CheckJWT middleware. A 403 response is sent when requests don't have the necessary
permissions.

```go
// setup verifier to use in middleware
verifier := &auth.Verifier{
    Resource: "https://cyberdyne-robot.com", // B's Resource URI
    TenantURL: "https://redventures.auth0.com", // TenantURL for environment (prod vs non-prod)
}

// setup authentication middleware
checkJWT := middleware.CheckJWT(verifier)

// setup authorization middleware by declaring the required permissions.
authorize := middleware.Authorize("read:billing", "write:billing")

// Register the middleware. NOTE: checkJWT runs first, followed by authorize.
http.ListenAndServe(":3000", checkJWT(authorize(mux)))
```

