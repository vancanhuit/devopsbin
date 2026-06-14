# AuthApi

All URIs are relative to */api/v1*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**getAuthMe**](AuthApi.md#getauthme) | **GET** /auth/me | Return the current authenticated user |
| [**postAuthLogin**](AuthApi.md#postauthlogin) | **POST** /auth/login | Log in with username and password |
| [**postAuthLogout**](AuthApi.md#postauthlogout) | **POST** /auth/logout | Log out of the current session |
| [**postAuthPasswordChange**](AuthApi.md#postauthpasswordchange) | **POST** /auth/password/change | Change the current user\&#39;s password |
| [**postAuthPasswordReset**](AuthApi.md#postauthpasswordreset) | **POST** /auth/password/reset | Reset a password using a reset token |
| [**postAuthPasswordResetRequest**](AuthApi.md#postauthpasswordresetrequest) | **POST** /auth/password/reset-request | Request a password-reset token |
| [**postAuthRegister**](AuthApi.md#postauthregister) | **POST** /auth/register | Register a new user |



## getAuthMe

> UserResponse getAuthMe()

Return the current authenticated user

Returns the user associated with the current session. Requires a valid session cookie. 

### Example

```ts
import {
  Configuration,
  AuthApi,
} from '';
import type { GetAuthMeRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new AuthApi();

  try {
    const data = await api.getAuthMe();
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters

This endpoint does not need any parameter.

### Return type

[**UserResponse**](UserResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | The current authenticated user. |  -  |
| **401** | No valid session is present. |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## postAuthLogin

> UserResponse postAuthLogin(loginRequest)

Log in with username and password

Verifies the credentials and opens an authenticated session: the response sets the session and CSRF cookies. A successful login rotates the session, issuing a fresh session id and CSRF token. 

### Example

```ts
import {
  Configuration,
  AuthApi,
} from '';
import type { PostAuthLoginRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new AuthApi();

  const body = {
    // LoginRequest
    loginRequest: ...,
  } satisfies PostAuthLoginRequest;

  try {
    const data = await api.postAuthLogin(body);
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters


| Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **loginRequest** | [LoginRequest](LoginRequest.md) |  | |

### Return type

[**UserResponse**](UserResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | The credentials were valid and a session was opened. The session and CSRF cookies are set via Set-Cookie.  |  * Set-Cookie - Sets the session and CSRF cookies. <br>  |
| **400** | The request body is missing or malformed. |  -  |
| **401** | The username or password is incorrect. |  -  |
| **423** | Too many failed login attempts. The account or client is temporarily locked; retry after the period indicated by the Retry-After header.  |  * Retry-After - Seconds to wait before retrying. <br>  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## postAuthLogout

> postAuthLogout(xCSRFToken)

Log out of the current session

Deletes the current session and clears the session and CSRF cookies. Requires a valid session cookie and a matching X-CSRF-Token header. 

### Example

```ts
import {
  Configuration,
  AuthApi,
} from '';
import type { PostAuthLogoutRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new AuthApi();

  const body = {
    // string | The CSRF token from the devopsbin_csrf cookie. Required in practice on state-changing requests to authenticated routes; a missing or mismatched token yields a 403 response. Must match the token bound to the current session.  (optional)
    xCSRFToken: 9f1c2a7b6e4d4f0a8c3b1d2e5f6a7b8c,
  } satisfies PostAuthLogoutRequest;

  try {
    const data = await api.postAuthLogout(body);
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters


| Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **xCSRFToken** | `string` | The CSRF token from the devopsbin_csrf cookie. Required in practice on state-changing requests to authenticated routes; a missing or mismatched token yields a 403 response. Must match the token bound to the current session.  | [Optional] [Defaults to `undefined`] |

### Return type

`void` (Empty response body)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **204** | The session was deleted and the session and CSRF cookies were cleared via Set-Cookie.  |  * Set-Cookie - Clears the session and CSRF cookies. <br>  |
| **401** | No valid session is present. |  -  |
| **403** | The CSRF token is missing or does not match the session. |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## postAuthPasswordChange

> UserResponse postAuthPasswordChange(passwordChangeRequest, xCSRFToken)

Change the current user\&#39;s password

Verifies the current password and sets a new one for the authenticated user. On success the session is rotated (a fresh session id and CSRF token are issued via Set-Cookie) and all other sessions for the user are revoked. Requires a valid session cookie and a matching X-CSRF-Token header. 

### Example

```ts
import {
  Configuration,
  AuthApi,
} from '';
import type { PostAuthPasswordChangeRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new AuthApi();

  const body = {
    // PasswordChangeRequest
    passwordChangeRequest: ...,
    // string | The CSRF token from the devopsbin_csrf cookie. Required in practice on state-changing requests to authenticated routes; a missing or mismatched token yields a 403 response. Must match the token bound to the current session.  (optional)
    xCSRFToken: 9f1c2a7b6e4d4f0a8c3b1d2e5f6a7b8c,
  } satisfies PostAuthPasswordChangeRequest;

  try {
    const data = await api.postAuthPasswordChange(body);
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters


| Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **passwordChangeRequest** | [PasswordChangeRequest](PasswordChangeRequest.md) |  | |
| **xCSRFToken** | `string` | The CSRF token from the devopsbin_csrf cookie. Required in practice on state-changing requests to authenticated routes; a missing or mismatched token yields a 403 response. Must match the token bound to the current session.  | [Optional] [Defaults to `undefined`] |

### Return type

[**UserResponse**](UserResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | The password was changed and the session was rotated. The session and CSRF cookies are refreshed via Set-Cookie.  |  * Set-Cookie - Refreshes the session and CSRF cookies. <br>  |
| **400** | The request body is missing or malformed. |  -  |
| **401** | No valid session is present. |  -  |
| **403** | The CSRF token is missing or does not match the session, or the current password is incorrect.  |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## postAuthPasswordReset

> MessageResponse postAuthPasswordReset(passwordResetRequest)

Reset a password using a reset token

Consumes a single-use reset token and sets a new password for the associated user. The token is invalidated on use and all of the user\&#39;s sessions are revoked. 

### Example

```ts
import {
  Configuration,
  AuthApi,
} from '';
import type { PostAuthPasswordResetRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new AuthApi();

  const body = {
    // PasswordResetRequest
    passwordResetRequest: ...,
  } satisfies PostAuthPasswordResetRequest;

  try {
    const data = await api.postAuthPasswordReset(body);
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters


| Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **passwordResetRequest** | [PasswordResetRequest](PasswordResetRequest.md) |  | |

### Return type

[**MessageResponse**](MessageResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | The password was reset. |  -  |
| **400** | The request body is missing or malformed. |  -  |
| **410** | The reset token is unknown, expired, or already used. |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## postAuthPasswordResetRequest

> PasswordResetResponse postAuthPasswordResetRequest(passwordResetRequestRequest)

Request a password-reset token

Issues a single-use, expiring password-reset token for the given username. To avoid leaking which usernames exist, the response is always 200 regardless of whether the user exists. In production the token would be emailed; for this demo it is returned in the response body when the user exists. 

### Example

```ts
import {
  Configuration,
  AuthApi,
} from '';
import type { PostAuthPasswordResetRequestRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new AuthApi();

  const body = {
    // PasswordResetRequestRequest
    passwordResetRequestRequest: ...,
  } satisfies PostAuthPasswordResetRequestRequest;

  try {
    const data = await api.postAuthPasswordResetRequest(body);
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters


| Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **passwordResetRequestRequest** | [PasswordResetRequestRequest](PasswordResetRequestRequest.md) |  | |

### Return type

[**PasswordResetResponse**](PasswordResetResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | The request was accepted. When the user exists, a reset token is included in the response.  |  -  |
| **400** | The request body is missing or malformed. |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## postAuthRegister

> UserResponse postAuthRegister(registerRequest)

Register a new user

Creates a user with the given username and password and opens an authenticated session: the response sets the session and CSRF cookies. A starter account is created for the new user in the same transaction. 

### Example

```ts
import {
  Configuration,
  AuthApi,
} from '';
import type { PostAuthRegisterRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new AuthApi();

  const body = {
    // RegisterRequest
    registerRequest: ...,
  } satisfies PostAuthRegisterRequest;

  try {
    const data = await api.postAuthRegister(body);
    console.log(data);
  } catch (error) {
    console.error(error);
  }
}

// Run the test
example().catch(console.error);
```

### Parameters


| Name | Type | Description  | Notes |
|------------- | ------------- | ------------- | -------------|
| **registerRequest** | [RegisterRequest](RegisterRequest.md) |  | |

### Return type

[**UserResponse**](UserResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **201** | The user was created and a session was opened. The session and CSRF cookies are set via Set-Cookie.  |  * Set-Cookie - Sets the session and CSRF cookies. <br>  |
| **400** | The request body is missing or malformed. |  -  |
| **409** | The username is already taken. |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

