let chart = null;
let allEntries = {};

// init chart
const ctx = document.getElementById('scoreChart').getContext('2d');
chart = new Chart(ctx, {
  type: 'bar',
  data: {
    labels: [],
    datasets: [{
      label: 'Score',
      data: [],
      backgroundColor: '#6C63D8',
      borderRadius: 6
    }]
  },
  options: {
    responsive: true,
    plugins: { legend: { display: false } },
    scales: {
      x: { ticks: { color: '#888' }, grid: { color: '#2a2a4a' } },
      y: { ticks: { color: '#888' }, grid: { color: '#2a2a4a' } }
    }
  }
});

function getLatencyClass(ms) {
  if (ms < 100) return 'good';
  if (ms < 300) return 'warning';
  return 'bad';
}

function updateLeaderboard(entries) {
  if (!Array.isArray(entries)) {
    allEntries[entries.contestant] = entries;
  } else {
    entries.forEach(e => allEntries[e.contestant] = e);
  }

  const sorted = Object.values(allEntries).sort((a, b) => b.score - a.score);

  document.getElementById('total-contestants').textContent = sorted.length;
  document.getElementById('top-tps').textContent = Math.round(Math.max(...sorted.map(e => e.tps)));
  document.getElementById('best-p99').textContent = Math.min(...sorted.map(e => e.p99)) + 'ms';
  document.getElementById('last-update').textContent = new Date().toLocaleTimeString();

  const tbody = document.getElementById('leaderboard-body');
  const maxScore = sorted[0]?.score || 1;

  tbody.innerHTML = sorted.map((e, i) => {
    const rank = i + 1;
    const rankClass = rank <= 3 ? `rank-${rank}` : '';
    const medal = rank === 1 ? '🥇' : rank === 2 ? '🥈' : rank === 3 ? '🥉' : rank;
    const barWidth = Math.round((e.score / maxScore) * 100);

    return `
      <tr>
        <td class="rank ${rankClass}">${medal}</td>
        <td class="contestant">${e.contestant}</td>
        <td class="score">${e.score.toFixed(1)}</td>
        <td class="${e.tps > 1000 ? 'good' : 'warning'}">${Math.round(e.tps)}</td>
        <td class="${getLatencyClass(e.p50)}">${e.p50}ms</td>
        <td class="${getLatencyClass(e.p90)}">${e.p90}ms</td>
        <td class="${getLatencyClass(e.p99)}">${e.p99}ms</td>
        <td class="${e.success_rate > 95 ? 'good' : 'bad'}">${e.success_rate.toFixed(1)}%</td>
        <td>
          <div class="score-bar">
            <div class="score-bar-fill" style="width:${barWidth}%"></div>
          </div>
        </td>
      </tr>`;
  }).join('');

  chart.data.labels = sorted.map(e => e.contestant);
  chart.data.datasets[0].data = sorted.map(e => e.score.toFixed(1));
  chart.update();
}

function connect() {
  const ws = new WebSocket('ws://localhost:3000/ws');

  ws.onopen = () => {
    document.getElementById('status').textContent = '● LIVE';
    document.getElementById('status').style.color = '#0F9B77';
    document.getElementById('status').style.borderColor = '#0F9B77';
    console.log('Connected to leaderboard!');
  };

  ws.onmessage = (event) => {
    try {
      const data = JSON.parse(event.data);
      updateLeaderboard(data);
    } catch(e) {
      console.error('Parse error:', e);
    }
  };

  ws.onclose = () => {
    document.getElementById('status').textContent = '● RECONNECTING...';
    document.getElementById('status').style.color = '#F5A623';
    setTimeout(connect, 2000);
  };

  ws.onerror = (err) => {
    console.error('WebSocket error:', err);
  };
}

connect();