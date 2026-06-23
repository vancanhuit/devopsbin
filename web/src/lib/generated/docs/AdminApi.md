# AdminApi

All URIs are relative to */api/v1*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**getAdminAccounts**](AdminApi.md#getadminaccounts) | **GET** /admin/accounts | List all accounts |
| [**getAdminTransfers**](AdminApi.md#getadmintransfers) | **GET** /admin/transfers | List the transfers ledger |
| [**getAdminUsers**](AdminApi.md#getadminusers) | **GET** /admin/users | List all users |
| [**postAdminUserPasswordReset**](AdminApi.md#postadminuserpasswordreset) | **POST** /admin/users/{id}/password-reset | Issue a password-reset token for a user |
| [**postAdminUserUnlock**](AdminApi.md#postadminuserunlock) | **POST** /admin/users/{id}/unlock | Clear a user\&#39;s login lockout |



## getAdminAccounts

> AccountListResponse getAdminAccounts()

List all accounts

Returns every account across all users, joined to the owner\&#39;s username. Requires a valid session whose user has the &#x60;admin&#x60; role. 

### Example

```ts
import {
  Configuration,
  AdminApi,
} from '';
import type { GetAdminAccountsRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new AdminApi();

  try {
    const data = await api.getAdminAccounts();
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

[**AccountListResponse**](AccountListResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | The list of accounts. |  -  |
| **401** | No valid session is present. |  -  |
| **403** | The session user is not an administrator. |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getAdminTransfers

> TransferListResponse getAdminTransfers()

List the transfers ledger

Returns the transfers ledger (most recent first), joined to the source and destination account names. Requires a valid session whose user has the &#x60;admin&#x60; role. 

### Example

```ts
import {
  Configuration,
  AdminApi,
} from '';
import type { GetAdminTransfersRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new AdminApi();

  try {
    const data = await api.getAdminTransfers();
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

[**TransferListResponse**](TransferListResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | The transfers ledger. |  -  |
| **401** | No valid session is present. |  -  |
| **403** | The session user is not an administrator. |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getAdminUsers

> UserListResponse getAdminUsers()

List all users

Returns every user (id, username, role, and creation time). Requires a valid session whose user has the &#x60;admin&#x60; role. 

### Example

```ts
import {
  Configuration,
  AdminApi,
} from '';
import type { GetAdminUsersRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new AdminApi();

  try {
    const data = await api.getAdminUsers();
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

[**UserListResponse**](UserListResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | The list of users. |  -  |
| **401** | No valid session is present. |  -  |
| **403** | The session user is not an administrator. |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## postAdminUserPasswordReset

> PasswordResetResponse postAdminUserPasswordReset(id, xCSRFToken)

Issue a password-reset token for a user

Mints a single-use, expiring password-reset token for the given user. In production the token would be emailed; for this demo it is returned in the response body. Requires a valid admin session and a matching X-CSRF-Token header. 

### Example

```ts
import {
  Configuration,
  AdminApi,
} from '';
import type { PostAdminUserPasswordResetRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new AdminApi();

  const body = {
    // string | The unique identifier of the user.
    id: 018f9d6b-cbbf-7b2d-9b5d-ab8dfbbd4bed,
    // string | The CSRF token from the devopsbin_csrf cookie. Required in practice on state-changing requests to authenticated routes; a missing or mismatched token yields a 403 response. Must match the token bound to the current session.  (optional)
    xCSRFToken: 9f1c2a7b6e4d4f0a8c3b1d2e5f6a7b8c,
  } satisfies PostAdminUserPasswordResetRequest;

  try {
    const data = await api.postAdminUserPasswordReset(body);
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
| **id** | `string` | The unique identifier of the user. | [Defaults to `undefined`] |
| **xCSRFToken** | `string` | The CSRF token from the devopsbin_csrf cookie. Required in practice on state-changing requests to authenticated routes; a missing or mismatched token yields a 403 response. Must match the token bound to the current session.  | [Optional] [Defaults to `undefined`] |

### Return type

[**PasswordResetResponse**](PasswordResetResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | A reset token was issued for the user. |  -  |
| **401** | No valid session is present. |  -  |
| **403** | The session user is not an administrator, or the X-CSRF-Token is missing or does not match the session.  |  -  |
| **404** | No user matches the given id. |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## postAdminUserUnlock

> postAdminUserUnlock(id, xCSRFToken)

Clear a user\&#39;s login lockout

Clears the brute-force failure counter and lock for the given user so they can log in again. IP-scoped locks are not affected. Requires a valid admin session and a matching X-CSRF-Token header. 

### Example

```ts
import {
  Configuration,
  AdminApi,
} from '';
import type { PostAdminUserUnlockRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new AdminApi();

  const body = {
    // string | The unique identifier of the user.
    id: 018f9d6b-cbbf-7b2d-9b5d-ab8dfbbd4bed,
    // string | The CSRF token from the devopsbin_csrf cookie. Required in practice on state-changing requests to authenticated routes; a missing or mismatched token yields a 403 response. Must match the token bound to the current session.  (optional)
    xCSRFToken: 9f1c2a7b6e4d4f0a8c3b1d2e5f6a7b8c,
  } satisfies PostAdminUserUnlockRequest;

  try {
    const data = await api.postAdminUserUnlock(body);
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
| **id** | `string` | The unique identifier of the user. | [Defaults to `undefined`] |
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
| **204** | The user\&#39;s login lockout was cleared. |  -  |
| **401** | No valid session is present. |  -  |
| **403** | The session user is not an administrator, or the X-CSRF-Token is missing or does not match the session.  |  -  |
| **404** | No user matches the given id. |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

