# RuntimeApi

All URIs are relative to */api/v1*

| Method | HTTP request | Description |
|------------- | ------------- | -------------|
| [**getLivez**](RuntimeApi.md#getlivez) | **GET** /livez | Liveness probe |
| [**getReadyz**](RuntimeApi.md#getreadyz) | **GET** /readyz | Readiness probe |
| [**getStartupz**](RuntimeApi.md#getstartupz) | **GET** /startupz | Startup probe |
| [**getVersion**](RuntimeApi.md#getversion) | **GET** /version | Build and version metadata |



## getLivez

> LivezResponse getLivez()

Liveness probe

Process-only liveness check. Do not check PostgreSQL or Redis here.

### Example

```ts
import {
  Configuration,
  RuntimeApi,
} from '';
import type { GetLivezRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new RuntimeApi();

  try {
    const data = await api.getLivez();
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

[**LivezResponse**](LivezResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | Process is alive. |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getReadyz

> ReadyzResponse getReadyz()

Readiness probe

Checks whether the application is ready to receive traffic.

### Example

```ts
import {
  Configuration,
  RuntimeApi,
} from '';
import type { GetReadyzRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new RuntimeApi();

  try {
    const data = await api.getReadyz();
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

[**ReadyzResponse**](ReadyzResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | Application is ready. |  -  |
| **503** | Application is not ready. |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getStartupz

> StartupzResponse getStartupz()

Startup probe

Checks whether application startup has completed.

### Example

```ts
import {
  Configuration,
  RuntimeApi,
} from '';
import type { GetStartupzRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new RuntimeApi();

  try {
    const data = await api.getStartupz();
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

[**StartupzResponse**](StartupzResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | Application startup is complete. |  -  |
| **503** | Application startup is still in progress. |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)


## getVersion

> VersionResponse getVersion()

Build and version metadata

### Example

```ts
import {
  Configuration,
  RuntimeApi,
} from '';
import type { GetVersionRequest } from '';

async function example() {
  console.log("🚀 Testing  SDK...");
  const api = new RuntimeApi();

  try {
    const data = await api.getVersion();
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

[**VersionResponse**](VersionResponse.md)

### Authorization

No authorization required

### HTTP request headers

- **Content-Type**: Not defined
- **Accept**: `application/json`


### HTTP response details
| Status code | Description | Response headers |
|-------------|-------------|------------------|
| **200** | Version metadata. |  -  |

[[Back to top]](#) [[Back to API list]](../README.md#api-endpoints) [[Back to Model list]](../README.md#models) [[Back to README]](../README.md)

