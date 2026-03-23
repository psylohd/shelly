use std::collections::HashMap;
use std::sync::{Arc, Mutex};
use std::time::{SystemTime, UNIX_EPOCH};

#[derive(Debug, Clone)]
pub struct Session {
    pub id: usize,
    pub session_type: String, // "netcat", "socat", etc.
    pub created: u64, // timestamp
    pub last_activity: u64,
    pub is_active: bool,
    pub listener_port: u16,
    pub target_ip: String,
}

impl Session {
    pub fn new(id: usize, session_type: &str, listener_port: u16, target_ip: &str) -> Self {
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("Time went backwards")
            .as_secs();
        
        Session {
            id,
            session_type: session_type.to_string(),
            created: now,
            last_activity: now,
            is_active: true,
            listener_port,
            target_ip: target_ip.to_string(),
        }
    }
    
    pub fn update_activity(&mut self) {
        let now = SystemTime::now()
            .duration_since(UNIX_EPOCH)
            .expect("Time went backwards")
            .as_secs();
        self.last_activity = now;
    }
    
    pub fn deactivate(&mut self) {
        self.is_active = false;
    }
}

pub struct SessionManager {
    sessions: Arc<Mutex<HashMap<usize, Session>>>,
    next_id: Arc<Mutex<usize>>,
}

impl SessionManager {
    pub fn new() -> Self {
        SessionManager {
            sessions: Arc::new(Mutex::new(HashMap::new())),
            next_id: Arc::new(Mutex::new(1)),
        }
    }
    
    pub fn create_session(&self, session_type: &str, listener_port: u16, target_ip: &str) -> usize {
        let mut next_id = self.next_id.lock().unwrap();
        let id = *next_id;
        *next_id += 1;
        
        let session = Session::new(id, session_type, listener_port, target_ip);
        
        let mut sessions = self.sessions.lock().unwrap();
        sessions.insert(id, session);
        
        id
    }
    
    pub fn get_session(&self, id: usize) -> Option<Session> {
        let sessions = self.sessions.lock().unwrap();
        sessions.get(&id).cloned()
    }
    
    pub fn update_session_activity(&self, id: usize) {
        let mut sessions = self.sessions.lock().unwrap();
        if let Some(session) = sessions.get_mut(&id) {
            session.update_activity();
        }
    }
    
    pub fn deactivate_session(&self, id: usize) {
        let mut sessions = self.sessions.lock().unwrap();
        if let Some(session) = sessions.get_mut(&id) {
            session.deactivate();
        }
    }
    
    pub fn list_sessions(&self) -> Vec<Session> {
        let sessions = self.sessions.lock().unwrap();
        sessions.values().cloned().collect()
    }
    
    pub fn kill_session(&self, id: usize) -> bool {
        let mut sessions = self.sessions.lock().unwrap();
        if sessions.remove(&id).is_some() {
            true
        } else {
            false
        }
    }
    
    pub fn get_next_id(&self) -> usize {
        let next_id = self.next_id.lock().unwrap();
        *next_id
    }
    
    // Public methods to access the internal locks
    pub fn lock_sessions(&self) -> std::sync::MutexGuard<std::collections::HashMap<usize, Session>> {
        self.sessions.lock().unwrap()
    }
    
    pub fn lock_next_id(&self) -> std::sync::MutexGuard<usize> {
        self.next_id.lock().unwrap()
    }
}

// Default session manager instance
lazy_static::lazy_static! {
    pub static ref SESSION_MANAGER: SessionManager = SessionManager::new();
}

