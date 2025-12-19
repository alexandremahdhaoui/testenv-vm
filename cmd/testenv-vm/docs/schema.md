# testenv-vm Configuration



> Full OpenAPI specification: [spec.openapi.yaml](../spec.openapi.yaml)

## Fields

### `artifactDir`

- **Type:** `string`
- **Required:** No
- **Description:** Directory for storing artifacts (keys, logs, etc.).

### `cleanupOnFailure`

- **Type:** `boolean`
- **Required:** No
- **Description:** Whether to clean up resources on failure. Defaults to true.

### `defaultBaseImage`

- **Type:** `string`
- **Required:** No
- **Description:** Default base image to use for VMs. Can be a well-known reference or HTTPS URL.

### `defaultProvider`

- **Type:** `string`
- **Required:** No
- **Description:** Name of the default provider to use when not specified.

### `imageCacheDir`

- **Type:** `string`
- **Required:** No
- **Description:** Directory for caching downloaded VM base images.

### `images`

- **Type:** `array of `
- **Required:** No
- **Description:** VM base images to download and cache.

### `keys`

- **Type:** `array of `
- **Required:** No
- **Description:** SSH key pair resources to create.

### `networks`

- **Type:** `array of `
- **Required:** No
- **Description:** Network infrastructure resources to create.

### `providers`

- **Type:** `array of `
- **Required:** Yes
- **Description:** Available providers for resource provisioning.

### `stateDir`

- **Type:** `string`
- **Required:** No
- **Description:** Directory for persisting environment state.

### `vms`

- **Type:** `array of `
- **Required:** No
- **Description:** Virtual machine resources to create.

