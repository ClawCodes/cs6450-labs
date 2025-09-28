## Server Side Modifications

### 1. Lock Management Architecture Changes

#### Earlier Version
```go
readSet sync.Map //map[string]map[uint64]*uint64
```

#### Current Version
```go
type LockInfo struct {
    readHolders map[uint64]bool
    writeHolder *uint64
}
locks sync.Map // map[string]*LockInfo
```

The current version introduces a dedicated `LockInfo` structure that separates read holders from write holders.

### 2. Concurrency Control Modifications

- **Earlier Version:** Uses `sync.Mutex` with a single lock for the entire service
- **Current Version:** Uses `sync.RWMutex` with per-key lock management

### 3. Lock Operation Methods

**Current Version** introduces dedicated methods:
- `acquireReadLock()` - Handles read lock acquisition
- `acquireWriteLock()` - Handles write lock acquisition  
- `releaseLocks()` - Manages lock release

These methods handle lock upgrade scenarios (read lock → write lock).

## Client Side Modifications

### 1. Error Handling Approach

#### Earlier Version
```go
if err != nil {
    _ = txn.Abort()
    return fmt.Errorf("server-side error raised: %w", err)
}
```

#### Current Version
```go
if strings.Contains(err.Error(), "Cannot acquire") || strings.Contains(err.Error(), "Abort:") {
    return fmt.Errorf("lock conflict: %w", err)
}
// Real error - abort transaction
_ = txn.Abort()
return fmt.Errorf("server-side error raised: %w", err)
```

The current version differentiates between lock conflicts and other types of errors.

### 2. WriteSet Cache Implementation

#### Earlier Version
```go
cachedVal := txn.writeSet[key]
if cachedVal != "" {
    return cachedVal, nil
}
```

#### Current Version
```go
cachedVal, exists := txn.writeSet[key]
if exists {
    return cachedVal, nil
}
```

The current version uses existence checking rather than empty string comparison.

### 3. Retry Mechanism

**Current Version** implements:
- Exponential backoff strategy
- Jitter mechanism
- Configurable retry counts

### 4. Bank Transfer Workload Addition

**Current Version** adds new functionality:
- `runTransferClient()` - Manages transfer operations
- `initAccounts()` - Sets up initial account state
- `performTransfer()` - Executes transfer logic
- `checkTotalBalance()` - Validates system consistency

This provides a complex testing scenario for ACID properties.

### 5. Deadlock Prevention Strategy

The current version implements lock ordering in transfer functionality:
```go
accounts := []int{src, dst}
sort.Ints(accounts)  // Consistent lock acquisition order
```

## Feature Comparison

### Lock Management

| Feature | Earlier Version | Current Version |
|---------|-------------|-------------|
| Lock Structure | Map-based with pointer approach | Dedicated LockInfo structure |
| Read Lock Support | Core implementation | Multiple concurrent readers |
| Write Lock Support | Core exclusion | Write exclusion with upgrade support |
| Lock Upgrade | Not available | Read-to-write lock upgrade |
| Lock Release | Direct deletion | Structured cleanup process |

### Transaction Processing

| Feature | Earlier Version | Current Version |
|---------|-------------|-------------|
| Error Classification | Uniform error handling | Categorized error responses |
| Retry Strategy | Simple counter-based | Exponential backoff with jitter |
| Write Set Caching | String comparison approach | Existence-based checking |
| Transaction Cleanup | Basic cleanup | Comprehensive state management |

## Architecture Differences

### Scalability Considerations

1. **Locking Granularity:** Different approaches to lock scope and management
2. **Lock Coordination:** Current version includes lock upgrade capabilities
3. **Client Scaling:** Enhanced multi-client coordination in current version

### Reliability Characteristics

1. **Error Recovery:** Different strategies for handling failures
2. **Resource Management:** Varied approaches to transaction cleanup
3. **Deadlock Handling:** Current version includes prevention mechanisms
4. **Retry Logic:** Different retry strategies between versions

### Monitoring Capabilities

1. **Metric Coverage:** Current version includes additional performance metrics
2. **System Visibility:** Enhanced observability in current version
3. **Error Tracking:** More detailed error categorization

## Implementation Approaches

The two versions represent different design philosophies and implementation strategies:

### Earlier Version Characteristics
- Straightforward implementation approach
- Unified error handling strategy
- Simplified lock management structure
- Focus on core functionality

### Current Version Characteristics
- Structured approach to lock management
- Differentiated error handling
- Additional testing scenarios
- Enhanced monitoring capabilities

## Technical Trade-offs

Each version makes different trade-offs:

- **Complexity vs. Functionality:** The current version includes additional features at the cost of increased complexity
- **Error Handling:** Different approaches to retry logic and error categorization
- **Testing Coverage:** Current version includes more comprehensive testing scenarios
- **Performance Monitoring:** Enhanced observability comes with additional overhead