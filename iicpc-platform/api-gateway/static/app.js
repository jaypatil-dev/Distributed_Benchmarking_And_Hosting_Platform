/*
****** WHAT THIS FILE DOES ******
* This is the main frontend JavaScript file for the IICPC platform.
* It handles all client-side logic for the submission portal.
*
* SECTIONS:
* - AUTH => login, register, logout functions
*   Communicates with /auth/login, /auth/register, /auth/logout endpoints
*   Stores session token in localStorage after successful login
*   Redirects admin to /admin dashboard, contestants to submission portal
*
* - PAGE NAVIGATION => switches between auth, submit and status pages
*   Single page application — no full page reloads between views
*
* - SUBMISSION => handles code submission flow
*   Supports both typing code directly and uploading a file
*   Tab key inserts 4 spaces instead of switching focus
*   Sends POST /submit with language and code
*
* - STATUS POLLING => auto-updates submission status every 3 seconds
*   Polls GET /status/:id every 3 seconds automatically
*   Stops polling when status reaches COMPLETED or ERROR
*   No manual refresh needed
*
* - SUBMISSIONS PAGE => loads and displays submission history
*   Fetches GET /submissions for logged in user
*   Clicking submission ID shows code in a modal popup
*
* - VIEW CODE MODAL => displays submitted code in a popup
*   Fetches full submission details including code
*   Shows language, status and full code with syntax formatting
*
* WHY LOCALSTORAGE FOR SESSION?
* localStorage persists across page refreshes — user stays logged in
* even if they close and reopen the browser tab.
* Token is sent as Authorization header with every API request.
* Server validates token against Redis session store on every request.
*
* WHY SINGLE PAGE APPLICATION?
* Switching between views without full page reloads gives a faster,
* more responsive experience. Auth page, submit page and status page
* are all in the same HTML file — shown/hidden via display:none/block.
*/

// current state
let activeTab = 'editor';
let uploadedCode = '';
let currentSubmissionId = '';
let selectedRole = 'contestant';
let statusInterval = null;

// ── AUTH ──────────────────────────────────────────

function switchAuthTab(tab) {
  document.querySelectorAll('.auth-tab').forEach(t => t.classList.remove('active'));
  event.target.classList.add('active');
  document.getElementById('login-form').style.display = tab === 'login' ? 'block' : 'none';
  document.getElementById('register-form').style.display = tab === 'register' ? 'block' : 'none';
}

function selectRole(role, btn) {
  selectedRole = role;
  document.querySelectorAll('.role-btn').forEach(b => b.classList.remove('active'));
  btn.classList.add('active');
  document.getElementById('secret-field').style.display = role === 'admin' ? 'block' : 'none';
}

async function login() {
  const username = document.getElementById('login-username').value.trim();
  const password = document.getElementById('login-password').value.trim();
  const errorDiv = document.getElementById('login-error');
  errorDiv.style.display = 'none';

  if (!username || !password) {
    errorDiv.textContent = 'Please enter username and password';
    errorDiv.style.display = 'block';
    return;
  }

  try {
    const response = await fetch('/auth/login', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ username, password })
    });

    const data = await response.json();

    if (!response.ok) {
      errorDiv.textContent = data.error || 'Login failed';
      errorDiv.style.display = 'block';
      return;
    }

    // save session
    localStorage.setItem('token', data.token);
    localStorage.setItem('username', data.username);
    localStorage.setItem('role', data.role);

    // redirect based on role
    if (data.role === 'admin') {
      window.location.href = '/admin';
    } else {
      showSubmitPage(data.username);
    }

  } catch (err) {
    errorDiv.textContent = 'Failed to connect to server';
    errorDiv.style.display = 'block';
  }
}

async function register() {
  const username = document.getElementById('reg-username').value.trim();
  const password = document.getElementById('reg-password').value.trim();
  const confirm = document.getElementById('reg-confirm').value.trim();
  const secretKey = document.getElementById('reg-secret').value.trim();
  const errorDiv = document.getElementById('register-error');
  errorDiv.style.display = 'none';

  if (!username || !password || !confirm) {
    errorDiv.textContent = 'Please fill all fields';
    errorDiv.style.display = 'block';
    return;
  }

  if (password !== confirm) {
    errorDiv.textContent = 'Passwords do not match';
    errorDiv.style.display = 'block';
    return;
  }

  if (selectedRole === 'admin' && !secretKey) {
    errorDiv.textContent = 'Please enter admin secret key';
    errorDiv.style.display = 'block';
    return;
  }

  try {
    const response = await fetch('/auth/register', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        username,
        password,
        role: selectedRole,
        secret_key: secretKey
      })
    });

    const data = await response.json();

    if (!response.ok) {
      errorDiv.textContent = data.error || 'Registration failed';
      errorDiv.style.display = 'block';
      return;
    }

    // show success and switch to login tab
    alert('Account created successfully! Please login.');
    document.getElementById('login-username').value = username;
    document.getElementById('login-password').value = password;
    switchAuthTab('login');

  } catch (err) {
    errorDiv.textContent = 'Failed to connect to server';
    errorDiv.style.display = 'block';
  }
}

async function logout() {
  const token = localStorage.getItem('token');
  if (token) {
    await fetch('/auth/logout', {
      method: 'POST',
      headers: { 'Authorization': token }
    });
  }
  localStorage.removeItem('token');
  localStorage.removeItem('username');
  localStorage.removeItem('role');
  window.location.href = '/';
}

// ── PAGE NAVIGATION ───────────────────────────────

function showSubmitPage(username) {
  document.getElementById('auth-page').style.display = 'none';
  document.getElementById('submit-page').style.display = 'block';
  document.getElementById('status-page').style.display = 'none';
  document.getElementById('logged-in-user').textContent = '👤 ' + username;
}

function showStatusPage(details) {
  document.getElementById('result-id').textContent = details.id;
  document.getElementById('result-contestant').textContent = details.contestant;
  document.getElementById('result-language').textContent = details.language.toUpperCase();
  document.getElementById('result-status').textContent = details.status.toUpperCase();
  document.getElementById('result-status').className = 'detail-value status-badge status-' + details.status;
  document.getElementById('result-time').textContent = details.time;
  document.getElementById('status-logged-user').textContent = '👤 ' + localStorage.getItem('username');

  document.getElementById('submit-page').style.display = 'none';
  document.getElementById('status-page').style.display = 'block';

  startPolling();
}

function goBack() {
  stopPolling();
  document.getElementById('submit-page').style.display = 'block';
  document.getElementById('status-page').style.display = 'none';
  currentSubmissionId = '';
}

function goToSubmissions() {
  stopPolling();
  window.location.href = '/my-submissions';
}

// ── SUBMISSION ────────────────────────────────────

function switchTab(tab, e) {
  activeTab = tab;
  document.querySelectorAll('.tab').forEach(t => t.classList.remove('active'));
  e.target.classList.add('active');
  document.getElementById('editor-tab').style.display = tab === 'editor' ? 'block' : 'none';
  document.getElementById('upload-tab').style.display = tab === 'upload' ? 'block' : 'none';
}

function handleFile(input) {
  const file = input.files[0];
  if (!file) return;
  document.getElementById('file-name').textContent = file.name;
  const reader = new FileReader();
  reader.onload = (e) => { uploadedCode = e.target.result; };
  reader.readAsText(file);
}

async function submitCode() {
  const token = localStorage.getItem('token');
  const errorDiv = document.getElementById('submit-error');
  errorDiv.style.display = 'none';

  const language = document.getElementById('language').value;
  const code = activeTab === 'editor'
    ? document.getElementById('code-editor').value.trim()
    : uploadedCode;

  if (!language) { errorDiv.textContent = 'Please select a language'; errorDiv.style.display = 'block'; return; }
  if (!code) { errorDiv.textContent = 'Please enter or upload your code'; errorDiv.style.display = 'block'; return; }

  const btn = document.getElementById('submit-btn');
  btn.disabled = true;
  btn.textContent = 'Submitting...';

  try {
    const response = await fetch('/submit', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        'Authorization': token
      },
      body: JSON.stringify({
        language,
        code,
        contestant: localStorage.getItem('username')
      })
    });

    const data = await response.json();

    if (!response.ok) {
      errorDiv.textContent = data.error || 'Submission failed';
      errorDiv.style.display = 'block';
      return;
    }

    currentSubmissionId = data.submission_id;

    showStatusPage({
      id: data.submission_id,
      contestant: localStorage.getItem('username'),
      language,
      status: data.status,
      time: new Date().toLocaleString()
    });

  } catch (err) {
    errorDiv.textContent = 'Failed to connect to server';
    errorDiv.style.display = 'block';
  } finally {
    btn.disabled = false;
    btn.textContent = 'Submit Solution →';
  }
}

// ── STATUS POLLING ────────────────────────────────

let statusInterval2 = null;

async function checkStatus() {
  if (!currentSubmissionId) return;
  const token = localStorage.getItem('token');

  try {
    const response = await fetch(`/status/${currentSubmissionId}`, {
      headers: { 'Authorization': token }
    });
    const data = await response.json();
    const status = data.status;

    const badge = document.getElementById('result-status');
    badge.textContent = status.toUpperCase();
    badge.className = 'detail-value status-badge status-' + status;

    if (status === 'completed' || status === 'error') {
      stopPolling();
    }

  } catch (err) {
    console.error('Status check failed:', err);
  }
}

function startPolling() {
  checkStatus();
  statusInterval2 = setInterval(checkStatus, 3000);
}

function stopPolling() {
  if (statusInterval2) {
    clearInterval(statusInterval2);
    statusInterval2 = null;
  }
}

// ── SUBMISSIONS PAGE ──────────────────────────────

async function checkAuthAndLoadSubmissions() {
  const token = localStorage.getItem('token');
  const username = localStorage.getItem('username');

  if (!token) {
    window.location.href = '/';
    return;
  }

  document.getElementById('sub-logged-user').textContent = '👤 ' + username;
  await loadSubmissions(token);
}

async function loadSubmissions(token) {
  try {
    const response = await fetch('/submissions', {
      headers: { 'Authorization': token }
    });

    const data = await response.json();
    const submissions = data.submissions || [];

    const subtitle = document.getElementById('sub-count');
    subtitle.textContent = `${submissions.length} submission${submissions.length !== 1 ? 's' : ''} found`;

    const tbody = document.getElementById('submissions-body');

    if (submissions.length === 0) {
      tbody.innerHTML = '<tr><td colspan="6" class="empty">No submissions yet — go submit your first solution!</td></tr>';
      return;
    }

    tbody.innerHTML = submissions.map((sub, i) => {
      const date = new Date(sub.created_at);
      const dateStr = date.toLocaleDateString();
      const timeStr = date.toLocaleTimeString();
      const badgeClass = `badge badge-${sub.status}`;

      return `
        <tr>
          <td>${i + 1}</td>
          <td style="color:#6C63D8;font-size:12px;cursor:pointer" onclick="viewCode('${sub.id}', '${sub.language}')">${sub.id}</td>
          <td>${sub.language.toUpperCase()}</td>
          <td><span class="${badgeClass}">${sub.status.toUpperCase()}</span></td>
          <td>${dateStr}</td>
          <td>${timeStr}</td>
        </tr>`;
    }).join('');

  } catch (err) {
    console.error('Failed to load submissions:', err);
  }
}

// ── TAB KEY IN EDITOR ─────────────────────────────

document.addEventListener('DOMContentLoaded', () => {
  const token = localStorage.getItem('token');
  const username = localStorage.getItem('username');
  const role = localStorage.getItem('role');

  if (token && username) {
    if (document.getElementById('auth-page')) {
      if (role === 'admin') {
        window.location.href = '/admin';
      } else {
        showSubmitPage(username);
      }
    }
  }

  // tab key in code editor
  const editor = document.getElementById('code-editor');
  if (editor) {
    editor.addEventListener('keydown', (e) => {
      if (e.key === 'Tab') {
        e.preventDefault();
        const start = editor.selectionStart;
        const end = editor.selectionEnd;
        editor.value = editor.value.substring(0, start) + '    ' + editor.value.substring(end);
        editor.selectionStart = editor.selectionEnd = start + 4;
      }
    });
  }
});

// ── VIEW CODE MODAL ───────────────────────────────

async function viewCode(submissionId, language) {
  const token = localStorage.getItem('token');
  try {
    const response = await fetch(`/status/${submissionId}`, {
      headers: { 'Authorization': token }
    });
    const data = await response.json();

    const modal = document.createElement('div');
    modal.style.cssText = `
      position:fixed;top:0;left:0;width:100%;height:100%;
      background:rgba(0,0,0,0.8);z-index:1000;
      display:flex;align-items:center;justify-content:center;
    `;

    modal.innerHTML = `
      <div style="background:#1a1a2e;border:1px solid #6C63D8;border-radius:12px;
                  padding:24px;width:80%;max-width:900px;max-height:80vh;overflow-y:auto;">
        <div style="display:flex;justify-content:space-between;margin-bottom:16px">
          <h3 style="color:#6C63D8">📄 ${submissionId}</h3>
          <button onclick="this.closest('div[style]').remove()"
                  style="background:#D85A30;padding:6px 14px;border-radius:6px;
                         font-size:12px;cursor:pointer;border:none;color:white">
            ✕ Close
          </button>
        </div>
        <div style="display:flex;gap:16px;margin-bottom:12px">
          <span style="color:#888;font-size:12px">Language: <span style="color:#0F9B77">${language.toUpperCase()}</span></span>
          <span style="color:#888;font-size:12px">Status: <span style="color:#0F9B77">${data.status?.toUpperCase()}</span></span>
        </div>
        <pre style="background:#0f0f1a;border:1px solid #2a2a4a;border-radius:8px;
                    padding:16px;overflow-x:auto;font-size:13px;line-height:1.6;
                    color:#e0e0e0;white-space:pre-wrap">${data.code || 'Code not available'}</pre>
      </div>
    `;

    modal.addEventListener('click', (e) => {
      if (e.target === modal) modal.remove();
    });

    document.body.appendChild(modal);

  } catch (err) {
    alert('Failed to load code');
  }
}