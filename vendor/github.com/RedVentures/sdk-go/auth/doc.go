/*
Package auth is a client library for Red Ventures' auth solution.

It simplifies requesting the JSON web tokens (JWTs) required to access protected services and
verifying JWTs on incoming requests. Behind the scenes, a cache is used to prevent
uneccesary latency. While this package cannot fetch tokens on behalf of users, it is still able to
verify them.

Service to service auth uses the OAuth 2.0 client credential grant flow. Users authentication is
handled with Open ID Connect (OIDC), which is why it's not possible for this library to obtain user
tokens.

See https://www.oauth.com/oauth2-servers/access-tokens/client-credentials/  and
http://openid.net/connect/ for more details.

Fetching Tokens

Request a token for service A to talk to protected service B. Service A only needs a single granter,
no matter how many other services it talks to.

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

Using the NewRequest helper makes dealing with the token directly uneccesary.

    granter := &auth.Granter{
        ClientID:     "ee58c91d-de3b-4bf1-917f-9f4c47c9de50", // A's client ID
        ClientSecret: "94bcf5c3-c911-4ede-bee3-256902540806", // A's client secret
        TenantURL: "https://redventures.auth0.com", // TenantID for environment (prod vs non-prod)
    }

    // make a new request function for B
    newRequest := granter.NewRequestFunc("https://cyberdyne-robot.com")

    // Build a new request for B. The authorization header will get set behind the scenes, and a token
    //will be requested if a valid one does not currently exist in the cache.
    req, err := newRequest("GET", endpointForServiceB, nil)

    // make the request
    client.Do(req)

Authenticating Requests

You can manually verify the token.

	// setup verifier
	verifier := &auth.Verifier
	{
		Resource: "https://cyberdyne-robot.com", // B's Resource URI
		TenantURL: "https://redventures.auth0.com", // TenantID for environment (prod vs non-prod)
    }

    // parse and verify the token
    parsed, err := verifier.VerifyToken(someJWTString)

But it's much easier to do it by using the provided middleware subpackage
	// setup verifier
	verifier := &auth.Verifier
	{
		Resource: "https://cyberdyne-robot.com", // B's Resource URI
		TenantURL: "https://redventures.auth0.com", // TenantID for environment (prod vs non-prod)
    }

    // setup middleware
    checkJWT := middleware.CheckJWT(verifier)

    // register middleware
    http.ListenAndServe(":3000", checkJWT(mux))

Authorizing Requests

You'll need to authenticate a request in order to authorize it. Authorization middleware must run
after CheckJWT middleware. A 403 response is sent when requests that don't have the necessary
permissions.

    // setup verifier to use in middleware
    verifier := &auth.Verifier{
        Resource: "https://cyberdyne-robot.com", // B's Resource URI
        TenantURL: "https://redventures.auth0.com", // TenantID for environment (prod vs non-prod)
    }

    // setup authentication middleware
    checkJWT := middleware.CheckJWT(verifier)

    // setup authorization middleware by declaring the required permissions.
    authorize := middleware.Authorize("read:billing", "write:billing")

    // register middleware
    http.ListenAndServe(":3000", checkJWT(authorize(mux)))
*/
package auth
