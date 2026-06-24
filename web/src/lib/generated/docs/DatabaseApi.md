# DatabaseApi

All URIs are relative to */api/v1*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**getAccounts**](DatabaseApi.md#getaccounts) | **GET** /accounts | List all accounts |
| [**postTransfer**](DatabaseApi.md#posttransfer) | **POST** /transfer | Transfer funds between two accounts |



## getAccounts

> AccountListResponse getAccounts()

List all accounts

Returns every account across all users (id, owner username, name, and balance) so a transfer source and destination can be chosen. Requires a valid session; any authenticated user may call it. 

### Example

```ts
import {
  Configuration,
  DatabaseApi,
} from '';
import type { GetAccountsRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new DatabaseApi();

  try {
    const data = await api.getAccounts();
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

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## postTransfer

> TransferResult postTransfer(transferRequest, xCSRFToken, isolation, holdMs)

Transfer funds between two accounts

Moves the requested amount from the source account to the destination account inside a single transaction. The caller must own the source account. The transaction runs at the chosen isolation level (serializable by default) and retries automatically on serialization conflicts. Requires a valid session and a matching X-CSRF-Token header. 

### Example

```ts
import {
  Configuration,
  DatabaseApi,
} from '';
import type { PostTransferRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new DatabaseApi();

  const body = {
    // TransferRequest
    transferRequest: ...,
    // string | The CSRF token from the devopsbin_csrf cookie. Required in practice on state-changing requests to authenticated routes; a missing or mismatched token yields a 403 response. Must match the token bound to the current session.  (optional)
    xCSRFToken: 9f1c2a7b6e4d4f0a8c3b1d2e5f6a7b8c,
    // 'serializable' | 'repeatable-read' | 'read-committed' | The transaction isolation level for the transfer. Defaults to `serializable` when omitted.  (optional)
    isolation: serializable,
    // number | Optional delay, in milliseconds, applied inside the transaction after locking the accounts. Used to widen the contention window and demonstrate isolation and serialization retries under concurrency.  (optional)
    holdMs: 0,
  } satisfies PostTransferRequest;

  try {
    const data = await api.postTransfer(body);
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
| **transferRequest** | [TransferRequest](TransferRequest.md) |  | |
| **xCSRFToken** | `string` | The CSRF token from the devopsbin_csrf cookie. Required in practice on state-changing requests to authenticated routes; a missing or mismatched token yields a 403 response. Must match the token bound to the current session.  | [Optional] [Defaults to `undefined`] |
| **isolation** | `serializable`, `repeatable-read`, `read-committed` | The transaction isolation level for the transfer. Defaults to &#x60;serializable&#x60; when omitted.  | [Optional] [Defaults to `&#39;serializable&#39;`] [Enum: serializable, repeatable-read, read-committed] |
| **holdMs** | `number` | Optional delay, in milliseconds, applied inside the transaction after locking the accounts. Used to widen the contention window and demonstrate isolation and serialization retries under concurrency.  | [Optional] [Defaults to `0`] |

### Return type

[**TransferResult**](TransferResult.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: `application/json`
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | The transfer committed; the resulting balances are returned. |  -  |
| **400** | The request body is missing or malformed, the amount is not positive, or the source and destination are the same account.  |  -  |
| **401** | No valid session is present. |  -  |
| **403** | The caller does not own the source account, or the X-CSRF-Token is missing or does not match the session.  |  -  |
| **404** | The source or destination account does not exist. |  -  |
| **409** | The source account has insufficient funds, or the transfer could not commit after exhausting the serialization-retry budget.  |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

