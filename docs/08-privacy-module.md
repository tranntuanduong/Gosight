# GoSight - Privacy Module

## 1. Overview

The Privacy Module ensures sensitive user data is protected throughout the GoSight pipeline. Privacy controls operate at multiple layers:

1. **SDK Layer** - Client-side masking and blocking
2. **Ingestor Layer** - Server-side validation and IP handling
3. **Storage Layer** - Data retention and anonymization
4. **API Layer** - Access control and data export

---

## 2. Privacy Levels

### Level 1: Essential (Default)

| Feature | Description |
|---------|-------------|
| Password masking | All `<input type="password">` auto-masked |
| Hidden inputs | All `<input type="hidden">` excluded |
| Credit card detection | Auto-mask 16-digit patterns |
| Input value privacy | Never capture actual input values |

### Level 2: Enhanced

| Feature | Description |
|---------|-------------|
| All inputs masked | Mask all form inputs |
| Selector blocking | Block specific CSS selectors |
| URL blocking | Skip recording on sensitive pages |
| IP anonymization | Truncate or hash IP addresses |

### Level 3: Maximum

| Feature | Description |
|---------|-------------|
| Text masking | Replace all text with placeholders |
| Regex masking | Custom patterns (email, phone, SSN) |
| Role-based replay | Restrict replay access by role |
| Data export/delete | GDPR compliance APIs |

---

## 3. SDK Privacy Configuration

### Full Configuration

```typescript
interface PrivacyConfig {
  // Input masking
  maskAllInputs: boolean;           // Default: true
  maskInputTypes: string[];         // Default: ['password', 'email', 'tel']

  // Selector-based controls
  maskSelectors: string[];          // CSS selectors to mask content
  blockSelectors: string[];         // CSS selectors to block from replay
  ignoreSelectors: string[];        // CSS selectors to ignore events

  // URL-based controls
  blockUrls: string[];              // URL patterns to not record
  allowUrls: string[];              // Only record these URLs (whitelist mode)

  // Text masking
  maskAllText: boolean;             // Replace all text with ****
  maskRegex: RegExp[];              // Custom regex patterns to mask

  // Network
  anonymizeIp: boolean;             // Truncate/hash IP address
  maskRequestBodies: boolean;       // Mask network request bodies
  maskResponseBodies: boolean;      // Mask network response bodies

  // Storage
  excludeFromStorage: string[];     // Field names to never store
}
```

### Configuration Examples

#### E-commerce Site

```typescript
GoSight.init({
  projectKey: 'gs_xxx',
  privacy: {
    maskAllInputs: true,
    blockSelectors: [
      '.payment-form',
      '#credit-card-section',
      '[data-sensitive]'
    ],
    blockUrls: [
      '/checkout/payment',
      '/account/billing'
    ],
    maskRegex: [
      /\b\d{4}[-\s]?\d{4}[-\s]?\d{4}[-\s]?\d{4}\b/g,  // Credit cards
      /\b\d{3}[-.]?\d{2}[-.]?\d{4}\b/g                 // SSN
    ]
  }
});
```

#### Healthcare Application

```typescript
GoSight.init({
  projectKey: 'gs_xxx',
  privacy: {
    maskAllInputs: true,
    maskAllText: true,  // HIPAA compliance
    blockSelectors: [
      '.patient-info',
      '.medical-records',
      '.prescription-details'
    ],
    blockUrls: [
      '/patients/*',
      '/records/*'
    ],
    anonymizeIp: true
  }
});
```

---

## 4. Data Attribute Controls

### Masking Content

```html
<!-- Mask text content in replay (shows as ****) -->
<div data-gosight-mask>
  Patient Name: John Doe
  SSN: 123-45-6789
</div>

<!-- Mask specific child elements -->
<div class="user-profile">
  <span data-gosight-mask>john.doe@email.com</span>
</div>
```

### Blocking from Replay

```html
<!-- Block entire section (shows placeholder) -->
<div data-gosight-block>
  <iframe src="payment-provider.com/form"></iframe>
</div>

<!-- Block sensitive images -->
<img data-gosight-block src="id-document.jpg" />
```

### Ignoring Events

```html
<!-- Don't track any events from this element -->
<div data-gosight-ignore>
  <input type="text" placeholder="Internal tool" />
  <button>Admin Action</button>
</div>
```

---

## 5. Input Masking Logic

### SDK Implementation

```typescript
class InputMasker {
  private config: PrivacyConfig;

  shouldMask(element: HTMLInputElement | HTMLTextAreaElement): boolean {
    // Always mask these types
    const alwaysMask = ['password', 'hidden'];
    if (alwaysMask.includes(element.type)) {
      return true;
    }

    // Check config flag
    if (this.config.maskAllInputs) {
      return true;
    }

    // Check specific types
    if (this.config.maskInputTypes?.includes(element.type)) {
      return true;
    }

    // Check CSS selectors
    if (this.matchesSelectors(element, this.config.maskSelectors)) {
      return true;
    }

    // Check data attribute
    if (element.closest('[data-gosight-mask]')) {
      return true;
    }

    // Check input name patterns
    const sensitiveNames = ['ssn', 'credit', 'card', 'cvv', 'password', 'secret'];
    const name = element.name?.toLowerCase() || '';
    if (sensitiveNames.some(s => name.includes(s))) {
      return true;
    }

    return false;
  }

  maskValue(value: string, element?: HTMLElement): string {
    // Apply regex patterns first
    let masked = value;
    for (const regex of this.config.maskRegex || []) {
      masked = masked.replace(regex, match => '█'.repeat(match.length));
    }

    // If no regex matched and we need to mask, use bullets
    if (masked === value && this.shouldMaskFully()) {
      return '•'.repeat(value.length);
    }

    return masked;
  }

  private matchesSelectors(element: Element, selectors?: string[]): boolean {
    if (!selectors?.length) return false;

    for (const selector of selectors) {
      try {
        if (element.matches(selector) || element.closest(selector)) {
          return true;
        }
      } catch {
        // Invalid selector, skip
      }
    }

    return false;
  }
}
```

### Common Regex Patterns

```typescript
const PRIVACY_PATTERNS = {
  // Credit card numbers
  creditCard: /\b\d{4}[-\s]?\d{4}[-\s]?\d{4}[-\s]?\d{4}\b/g,

  // Social Security Numbers
  ssn: /\b\d{3}[-.]?\d{2}[-.]?\d{4}\b/g,

  // Email addresses
  email: /\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b/g,

  // Phone numbers (various formats)
  phone: /\b(\+\d{1,3}[-.\s]?)?\(?\d{3}\)?[-.\s]?\d{3}[-.\s]?\d{4}\b/g,

  // IP addresses
  ipv4: /\b\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3}\b/g,

  // Date of birth patterns
  dob: /\b(0?[1-9]|1[0-2])[\/\-](0?[1-9]|[12]\d|3[01])[\/\-](\d{2}|\d{4})\b/g,

  // API keys / tokens (generic)
  apiKey: /\b[A-Za-z0-9_-]{32,}\b/g
};
```

---

## 6. rrweb Privacy Integration

### Recording Configuration

```typescript
import { record } from 'rrweb';

const stopRecording = record({
  emit(event) {
    // Send to buffer
    eventBuffer.add(event);
  },

  // Privacy options
  maskAllInputs: config.privacy.maskAllInputs,

  maskInputOptions: {
    password: true,
    email: config.privacy.maskAllInputs,
    tel: config.privacy.maskAllInputs,
    text: config.privacy.maskAllInputs,
    textarea: config.privacy.maskAllInputs,
  },

  // Block certain elements entirely
  blockClass: 'gosight-block',
  blockSelector: config.privacy.blockSelectors?.join(', '),

  // Ignore elements (don't record)
  ignoreClass: 'gosight-ignore',

  // Mask text content
  maskTextClass: 'gosight-mask',
  maskTextSelector: config.privacy.maskSelectors?.join(', '),

  // Custom masking function
  maskTextFn: (text, element) => {
    // Apply regex patterns
    let masked = text;
    for (const pattern of config.privacy.maskRegex || []) {
      masked = masked.replace(pattern, match => '█'.repeat(match.length));
    }

    // Full text masking mode
    if (config.privacy.maskAllText) {
      return '█'.repeat(text.length);
    }

    return masked;
  },

  // Custom input masking function
  maskInputFn: (text, element) => {
    if (inputMasker.shouldMask(element)) {
      return '•'.repeat(text.length);
    }
    return text;
  },

  // Don't record canvas content (potential PII in images)
  recordCanvas: false,

  // Sampling for performance
  sampling: {
    mousemove: true,
    mouseInteraction: true,
    scroll: 150,
    media: 800,
    input: 'last',
  },

  // Inline stylesheets and images
  inlineStylesheet: true,
  inlineImages: false,  // Don't inline (privacy + size)
});
```

---

## 7. URL Blocking

### SDK Implementation

```typescript
class URLBlocker {
  private blockPatterns: RegExp[];
  private allowPatterns: RegExp[];

  constructor(config: PrivacyConfig) {
    this.blockPatterns = this.compilePatterns(config.blockUrls);
    this.allowPatterns = this.compilePatterns(config.allowUrls);
  }

  shouldRecord(url: string): boolean {
    const path = new URL(url).pathname;

    // Whitelist mode
    if (this.allowPatterns.length > 0) {
      return this.allowPatterns.some(p => p.test(path));
    }

    // Blacklist mode
    if (this.blockPatterns.some(p => p.test(path))) {
      return false;
    }

    return true;
  }

  private compilePatterns(patterns?: string[]): RegExp[] {
    if (!patterns) return [];

    return patterns.map(pattern => {
      // Convert glob-like patterns to regex
      const escaped = pattern
        .replace(/[.+^${}()|[\]\\]/g, '\\$&')  // Escape special chars
        .replace(/\*/g, '.*')                    // * -> .*
        .replace(/\?/g, '.');                    // ? -> .

      return new RegExp(`^${escaped}$`);
    });
  }
}
```

### Usage

```typescript
// Block patterns
blockUrls: [
  '/checkout/*',           // All checkout pages
  '/account/billing',      // Specific page
  '/admin/*',              // Admin section
  '*/payment*',            // Any URL with "payment"
]

// Or whitelist mode
allowUrls: [
  '/',                     // Homepage
  '/products/*',           // Product pages
  '/blog/*',               // Blog
]
```

---

## 8. IP Anonymization

### Ingestor Implementation

```go
type IPAnonymizer struct {
    method string // "truncate", "hash", "remove"
}

func (a *IPAnonymizer) Anonymize(ip string) string {
    switch a.method {
    case "truncate":
        return a.truncate(ip)
    case "hash":
        return a.hash(ip)
    case "remove":
        return ""
    default:
        return ip
    }
}

func (a *IPAnonymizer) truncate(ip string) string {
    parsed := net.ParseIP(ip)
    if parsed == nil {
        return ""
    }

    if parsed.To4() != nil {
        // IPv4: 192.168.1.xxx
        parts := strings.Split(ip, ".")
        parts[3] = "0"
        return strings.Join(parts, ".")
    }

    // IPv6: 2001:db8:85a3::0000
    parts := strings.Split(ip, ":")
    if len(parts) >= 4 {
        for i := 4; i < len(parts); i++ {
            parts[i] = "0"
        }
    }
    return strings.Join(parts, ":")
}

func (a *IPAnonymizer) hash(ip string) string {
    // One-way hash for grouping without identifying
    h := sha256.Sum256([]byte(ip + "salt"))
    return fmt.Sprintf("anon_%x", h[:8])
}
```

---

## 9. Server-Side Validation

### Ingestor Privacy Enforcement

```go
type PrivacyEnforcer struct {
    projectSettings map[string]PrivacySettings
}

func (e *PrivacyEnforcer) EnforcePrivacy(projectID string, event *Event) error {
    settings := e.projectSettings[projectID]

    // Validate no raw input values
    if event.Type == "input_change" {
        if event.Payload.Value != "" && !event.Payload.IsMasked {
            return errors.New("raw input values not allowed")
        }
    }

    // Enforce IP anonymization
    if settings.AnonymizeIP {
        event.Metadata.IP = e.anonymizer.Anonymize(event.Metadata.IP)
    }

    // Validate blocked URLs
    if e.isBlockedURL(settings, event.Page.URL) {
        return errors.New("event from blocked URL")
    }

    // Strip excluded fields
    for _, field := range settings.ExcludeFields {
        delete(event.Custom, field)
    }

    return nil
}
```

---

## 10. Data Retention & Deletion

### GDPR Compliance APIs

```go
// DELETE /api/v1/projects/:id/users/:userId/data
func (h *Handler) DeleteUserData(w http.ResponseWriter, r *http.Request) {
    projectID := chi.URLParam(r, "id")
    userID := chi.URLParam(r, "userId")

    // Delete from ClickHouse
    err := h.clickhouse.Exec(ctx, `
        ALTER TABLE events DELETE
        WHERE project_id = ? AND user_id = ?
    `, projectID, userID)

    // Delete sessions
    err = h.clickhouse.Exec(ctx, `
        ALTER TABLE sessions DELETE
        WHERE project_id = ? AND user_id = ?
    `, projectID, userID)

    // Delete replay data
    err = h.clickhouse.Exec(ctx, `
        ALTER TABLE replay_chunks DELETE
        WHERE project_id = ? AND session_id IN (
            SELECT session_id FROM sessions
            WHERE project_id = ? AND user_id = ?
        )
    `, projectID, projectID, userID)

    // Log deletion for audit
    h.auditLog.Log(AuditEvent{
        Type:      "data_deletion",
        ProjectID: projectID,
        UserID:    userID,
        Timestamp: time.Now(),
    })
}

// GET /api/v1/projects/:id/users/:userId/data/export
func (h *Handler) ExportUserData(w http.ResponseWriter, r *http.Request) {
    // Export all data for a user (GDPR data portability)
    // Returns JSON with all events, sessions, etc.
}
```

---

## 11. Role-Based Replay Access

### Access Control

```go
type ReplayAccessControl struct {
    db *sql.DB
}

type ReplayPermission string

const (
    ReplayFull    ReplayPermission = "full"      // See everything
    ReplayMasked  ReplayPermission = "masked"    // Masked sensitive data
    ReplayNone    ReplayPermission = "none"      // No replay access
)

func (ac *ReplayAccessControl) CanViewReplay(userID, projectID, sessionID string) ReplayPermission {
    // Get user role
    var role string
    ac.db.QueryRow(`
        SELECT role FROM project_members
        WHERE user_id = ? AND project_id = ?
    `, userID, projectID).Scan(&role)

    switch role {
    case "owner", "admin":
        return ReplayFull
    case "member":
        return ReplayMasked
    case "viewer":
        return ReplayNone
    default:
        return ReplayNone
    }
}
```

### Replay Player Integration

```typescript
// Dashboard replay player
async function loadReplay(sessionId: string) {
  const response = await api.get(`/sessions/${sessionId}/replay`);

  // Check permission level
  if (response.permission === 'masked') {
    // Enable client-side masking during playback
    replayer.setMaskingEnabled(true);
  }

  if (response.permission === 'none') {
    throw new Error('No replay access');
  }

  return response.data;
}
```

---

## 12. Privacy Checklist

### Before Launch

- [ ] Configure `maskAllInputs: true`
- [ ] Add sensitive selectors to `blockSelectors`
- [ ] Add sensitive URLs to `blockUrls`
- [ ] Enable `anonymizeIp` if required
- [ ] Test replay to verify masking works
- [ ] Configure role-based replay access
- [ ] Set up data retention policies
- [ ] Document privacy policy for users

### Compliance

| Regulation | Requirements |
|------------|--------------|
| **GDPR** | Data export, deletion, consent |
| **CCPA** | Opt-out, data disclosure |
| **HIPAA** | Full text masking, audit logs |
| **PCI-DSS** | Credit card masking, no storage |

---

## 13. References

- [SDK Specification](./04-sdk-specification.md)
- [Session Replay](./09-session-replay.md)
- [API Specification](./05-api-specification.md)
