/*
****** WHAT THIS FILE DOES ******
* This file handles all admin dashboard functionality.
* 
* FUNCTIONS:
* - checkAdminAuth() => verifies user is logged in as admin
* - loadStats() => fetches and displays platform statistics
* - loadUsers() => fetches and displays all registered users
* - loadSubmissions() => fetches and displays all submissions
* - deleteUser() => deletes a user account
* - triggerTest() => manually triggers a re-test for a submission
* - switchTab() => switches between users and submissions tabs
* - logout() => logs out and redirects to login page
*/

// check admin auth on page load
window.onload = () => {
  checkAdminAuth();
};

function checkAdminAuth() {
  const token = localStorage.getItem('token');
  const role = localStorage.getItem('role');
  const username = localStorage.getItem('username');

  if (!token || role !== 'admin') {
    window.location.href = '/';
    return;
  }

  document.getElementById('admin-username').textContent = '🔧 ' + username;

  // load all data
  loadStats();
  loadUsers();
}

async function loadStats() {
  const token = localStorage.getItem('token');

  try {
    const response = await fetch('/admin/stats', {
      headers: { 'Authorization': token }
    });

    const data = await response.json();

    document.getElementById('total-users').textContent = data.total_users || 0;
    document.getElementById('total-contestants').textContent = data.total_contestants || 0;
    document.getElementById('total-submissions').textContent = data.total_submissions || 0;
    document.getElementById('top-contestant').textContent =
      data.top_contestant !== 'none'
        ? `${data.top_contestant} (${data.top_score?.toFixed(1)})`
        : 'none yet';

  } catch (err) {
    console.error('Failed to load stats:', err);
  }
}

async function loadUsers() {
  const token = localStorage.getItem('token');

  try {
    const response = await fetch('/admin/users', {
      headers: { 'Authorization': token }
    });

    const data = await response.json();
    const users = data.users || [];

    const tbody = document.getElementById('users-body');

    if (users.length === 0) {
      tbody.innerHTML = '<tr><td colspan="6" class="empty">No users registered yet</td></tr>';
      return;
    }

    tbody.innerHTML = users.map((user, i) => {
      const date = new Date(user.created_at).toLocaleString();
      const roleClass = user.role === 'admin' ? 'role-admin' : 'role-contestant';
      const deleteBtn = user.username !== 'admin'
        ? `<button class="delete-btn" onclick="deleteUser('${user.username}')">🗑 Delete</button>`
        : `<span style="color:#444;font-size:11px">protected</span>`;

      return `
        <tr>
          <td>${i + 1}</td>
          <td style="font-weight:bold">${user.username}</td>
          <td class="${roleClass}">${user.role.toUpperCase()}</td>
          <td style="color:#6C63D8">${user.submission_count}</td>
          <td style="font-size:12px;color:#888">${date}</td>
          <td>${deleteBtn}</td>
        </tr>`;
    }).join('');

  } catch (err) {
    console.error('Failed to load users:', err);
  }
}

async function loadSubmissions() {
  const token = localStorage.getItem('token');

  try {
    const response = await fetch('/admin/submissions', {
      headers: { 'Authorization': token }
    });

    const data = await response.json();
    const submissions = data.submissions || [];

    const tbody = document.getElementById('submissions-body');

    if (submissions.length === 0) {
      tbody.innerHTML = '<tr><td colspan="7" class="empty">No submissions yet</td></tr>';
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
          <td style="color:#6C63D8;font-size:11px">${sub.id}</td>
          <td style="font-weight:bold">${sub.contestant}</td>
          <td>${sub.language.toUpperCase()}</td>
          <td><span class="${badgeClass}">${sub.status.toUpperCase()}</span></td>
          <td style="font-size:12px;color:#888">${dateStr} ${timeStr}</td>
          <td>
            <button class="trigger-btn" onclick="triggerTest('${sub.id}')">
              ▶ Re-test
            </button>
          </td>
        </tr>`;
    }).join('');

  } catch (err) {
    console.error('Failed to load submissions:', err);
  }
}

async function deleteUser(username) {
  if (!confirm(`Are you sure you want to delete user "${username}"? This cannot be undone.`)) {
    return;
  }

  const token = localStorage.getItem('token');

  try {
    const response = await fetch(`/admin/users/${username}`, {
      method: 'DELETE',
      headers: { 'Authorization': token }
    });

    const data = await response.json();

    if (response.ok) {
      alert(`User "${username}" deleted successfully!`);
      loadUsers();
      loadStats();
    } else {
      alert(`Error: ${data.error}`);
    }

  } catch (err) {
    alert('Failed to delete user');
  }
}

async function triggerTest(submissionId) {
  const token = localStorage.getItem('token');

  try {
    const response = await fetch(`/admin/trigger/${submissionId}`, {
      method: 'POST',
      headers: { 'Authorization': token }
    });

    const data = await response.json();

    if (response.ok) {
      alert(`Re-test triggered for ${data.contestant}!\nCheck the leaderboard for updated scores.`);
      loadSubmissions();
    } else {
      alert(`Error: ${data.error}`);
    }

  } catch (err) {
    alert('Failed to trigger test');
  }
}

function switchTab(tab, btn) {
  document.querySelectorAll('.admin-tab').forEach(t => t.classList.remove('active'));
  btn.classList.add('active');

  document.getElementById('users-section').style.display = tab === 'users' ? 'block' : 'none';
  document.getElementById('submissions-section').style.display = tab === 'submissions' ? 'block' : 'none';

  if (tab === 'submissions') loadSubmissions();
  if (tab === 'users') loadUsers();
}