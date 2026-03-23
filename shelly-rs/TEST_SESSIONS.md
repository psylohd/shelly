# Shelly Session Management Test

This document demonstrates how the session management system works in Shelly.

## Overview

Shelly now includes a session management system that allows users to:
1. List active sessions
2. Switch between sessions
3. Kill sessions
4. Background sessions with Ctrl+Z

## Session Data Structure

Each session tracks:
- ID (numeric identifier)
- Type (netcat, socat, etc.)
- Creation timestamp
- Last activity timestamp
- Active status
- Listener port
- Target IP

## Usage Examples

### Starting a Listener

```bash
# Start a netcat listener on port 4444
./shelly -l 0.0.0.0 -p 4444 -s nc

# Start a socat listener on port 4444
./shelly -l 0.0.0.0 -p 4444 -s socat
```

### Session Management Commands

Once a listener is running and connections are established, users can:

#### List Sessions
```
shelly> sessions
```

Output:
```
Id  Type        Target              Status
--------------------------------------------------
1   netcat      192.168.1.100:4444  Active
2   socat       192.168.1.101:4444  Active
3   netcat      192.168.1.102:4444  Inactive
```

#### Switch Sessions
```
shelly> switch 2
[*] Switching to session 2
```

#### Kill Sessions
```
shelly> kill 1
[*] Session 1 killed
```

#### Background Current Session
While in an active session, press Ctrl+Z to background it and return to the shelly prompt.

## Implementation Details

### Session Tracking

Sessions are tracked using a global `SESSION_MANAGER` instance that:
- Assigns sequential numeric IDs to new sessions
- Stores session metadata in a HashMap
- Provides thread-safe access via Mutex
- Automatically deactivates sessions when connections end

### Backgrounding Sessions

When Ctrl+Z is pressed:
1. A signal handler sets a global flag
2. The shell implementation checks this flag in its main loop
3. When detected, it updates session activity and returns control to the shelly prompt
4. The session remains active but is suspended (in a full implementation)

### Automatic Listener Recreation

Listeners automatically restart when a connection terminates, allowing them to accept new connections without manual intervention.

## Future Improvements

1. Add proper session suspension/resume functionality
2. Implement session persistence across restarts
3. Add more detailed session information (commands executed, etc.)
4. Implement session sharing between multiple shelly instances
5. Add session tagging and filtering capabilities

## Conclusion

The session management system provides a Metasploit-like experience for managing multiple reverse shell connections, making it easier to handle multiple compromised systems during penetration testing engagements.