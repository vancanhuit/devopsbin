# StatusApi

All URIs are relative to */api/v1*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**getStatus**](StatusApi.md#getstatus) | **GET** /status/{code} | Return a given HTTP status code |



## getStatus

> StatusResponse getStatus(code)

Return a given HTTP status code

Responds with the HTTP status code provided in the path. The code must be a valid HTTP status code in the range 100-599; any other value yields a 400 response. 

### Example

```ts
import {
  Configuration,
  StatusApi,
} from '';
import type { GetStatusRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new StatusApi();

  const body = {
    // number | The HTTP status code to return (100-599).
    code: 200,
  } satisfies GetStatusRequest;

  try {
    const data = await api.getStatus(body);
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
| **code** | `number` | The HTTP status code to return (100-599). | [Defaults to `undefined`] |

### Return type

[**StatusResponse**](StatusResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **400** | The requested status code is outside the valid range. |  -  |
| **0** | The HTTP status code requested by the caller. |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

