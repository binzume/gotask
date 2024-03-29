"use strict";

const apiUrl = '';

/**
 * @param {string} tag 
 * @param {string[] | string | Node[] | any} children 
 * @param {object | function} attrs
 * @returns {HTMLElement}
 */
let mkEl = (tag, children, attrs) => {
	let el = document.createElement(tag);
	children && el.append(...[children].flat(999));
	attrs instanceof Function ? attrs(el) : (attrs && Object.assign(el, attrs));
	return el;
};

function formatTime(t) {
	t = t / 1000;
	return "" + (t / 60 | 0) + ":" + ("0" + (t % 60 | 0)).substr(-2);
}

function formatDate(s) {
	let t = new Date(s);
	if (t.getTime() <= 0) {
		return "";
	}
	let d2 = n => (n > 9 ? "" : "0") + n;
	return [t.getFullYear(), d2(t.getMonth() + 1), d2(t.getDate())].join("-") + " " +
		[d2(t.getHours()), d2(t.getMinutes())].join(":");
}

class TaskView {
	constructor() {
		this.currentTask = null;
		this.currentLog = null;

		document.getElementById('task-start-button').addEventListener('click', (e) => {
			e.preventDefault();
			this.startTask();
		});

		document.getElementById('task-refresh-button').addEventListener('click', (e) => {
			e.preventDefault();
			this.updateTask(taskView.currentTask);
			this.updateTaskLog(taskView.currentLog);
		});

		let searchTimeout = null;
		let filterEl = document.getElementById('task-filter');
		filterEl.addEventListener('input', (ev) => {
			clearTimeout(searchTimeout);
			searchTimeout = setTimeout(() => {
				this.setTaskFilter(filterEl.value);
			}, 300);
		});
	}

	updateGraph(run) {
		let svg = document.getElementById('task-graph');
		let graph = new TaskGraph();
		svg.innerHTML = '';
		if (!run || !run.task) {
			return;
		}

		let steps = run.task.steps;
		if (!steps || !steps.length) {
			steps = [run.task];
		}
		for (let step of steps) {
			let color = 'white';
			if (step.status == 'queued') {
				color = '#aaa';
			} else if (step.status == 'running') {
				color = '#8f8';
			} else if (step.status == 'finished') {
				color = '#6d6';
			} else if (step.status == 'success') {
				color = '#6d6';
			} else if (step.status == 'failed') {
				color = '#f00';
			} else if (step.status == 'canceled') {
				color = '#ff6';
			}
			let time = step.finishedAt ? '(' + formatTime(step.finishedAt - step.startedAt) + ')' : '';
			let o = { id: step.name, type: 'task', o: step, srcIds: step.depends || [], text: (step.status || '') + time, lane: 0, connectColor: 'black', color: color };
			if (graph.ids[o.srcIds[0]]) {
				o.lane = graph.ids[o.srcIds[0]].lane + 1;
			}
			graph.add(o);
		}
		graph.build(svg, (node) => {
			this.updateTaskLog(node.o.logFile);
			this.updateTaskInfo(node.o);
		});
	}

	async stopTask(taskId, runId) {
		let data = new FormData();
		data.append("action", "stop");
		data.append("runId", runId);
		let res = await fetch(apiUrl + 'tasks/' + taskId, { method: "POST", body: data });
		if (!res.ok) {
			return;
		}
		await res.json();
		setTimeout(() => this.updateTask(taskId), 200);
	}

	async startTask(taskId) {
		if (!taskId) {
			taskId = this.currentTask;
		}
		let data = new FormData();
		data.append("action", "start");
		// TODO: edit variables
		// data.append("VARS.TEST", "test");
		let res = await fetch(apiUrl + 'tasks/' + taskId, { method: "POST", body: data });
		if (!res.ok) {
			return;
		}
		setTimeout(() => this.updateTask(taskId), 0);
	}

	updateTaskInfo(t) {
		let infoEl = document.getElementById('task-info');
		infoEl.innerHTML = '';
		if (!t) {
			return;
		}
		infoEl.append(
			mkEl('span', t.name, { className: 'task-name' }),
			" : ",
			mkEl('span', t.startedAt ? formatDate(t.startedAt) : '', { className: 'task-date' }),
			" - ",
			mkEl('span', t.finishedAt ? formatDate(t.finishedAt) : '', { className: 'task-date' }),
			" ",
			mkEl('span', t.desc, { className: 'task-desc' }),
		);
		if (t.message) {
			infoEl.append(mkEl('span', t.message, { className: 'task-errormessage' }));
		}
	}

	async updateTaskLog(logfile) {
		let logEl = document.getElementById('task-log');
		if (logfile != this.currentLog) {
			logEl.innerText = '';
		}
		this.currentLog = logfile;
		if (!logfile) {
			logEl.style.display = 'none';
			return;
		}
		logEl.style.display = 'block';
		let res = await fetch(apiUrl + 'tasklogs/' + logfile);
		if (!res.ok) {
			logEl.innerText = 'Log not found';
			return;
		}
		logEl.innerText = await res.text();
		logEl.scrollTop = logEl.scrollHeight;
	}

	selectRun(run) {
		if  (run?.task?.steps == null && run?.task?.logFile) {
			this.updateTaskLog(run.task.logFile);
		} else {
			this.updateTaskLog(null);
		}
		if (run != null) {
			this.updateTaskInfo(run.task);
		} else {
			let infoEl = document.getElementById('task-info');
			infoEl.innerText = 'No runs';
		}
	}

	async updateTask(taskId) {
		let titleEl = document.getElementById('task-title');
		let historyEl = document.getElementById('task-history');
		if (this.currentTask != taskId) {
			titleEl.innerText = taskId;
			historyEl.innerText = '';
			this.updateTaskInfo(null);
			this.updateTaskLog(null);
			this.currentTask = taskId;
		}
		if (!taskId) {
			return;
		}

		let res = await fetch(apiUrl + 'tasks/' + taskId);
		if (!res.ok) {
			return;
		}
		let taskRes = await res.json();

		let lastRun = taskRes.recent && taskRes.recent[0];
		this.updateGraph(lastRun || taskRes);
		this.selectRun(lastRun);

		historyEl.innerText = '';
		titleEl.innerText = taskId;
		if (taskRes.schedule) {
			titleEl.innerText += `(${taskRes.schedule.spec})`;
		}
		for (let log of taskRes.recent || []) {
			let t = log.task;
			let start = t.startedAt ? formatDate(t.startedAt) : "wait";
			let time = t.finishedAt ? formatTime(t.finishedAt - t.startedAt) : '';
			let el = mkEl('li', [
				mkEl('span', start, { className: 'task-date' }),
				mkEl('span', t.status, { className: 'status status-' + t.status }),
			], { className: 'log', title: 'Run:' + log.runId });
			for (let st of t.steps || []) {
				el.append(mkEl('span', '.', { className: 'status-' + st.status }));
			}
			el.append(mkEl('span', ['(', time, ')'], { className: 'task-time' }));
			if (t.status == 'running' || t.status == 'queued') {
				el.append(mkEl('button', '■', {
					onclick: (ev) => {
						if (confirm(`Stop ${taskId}?`)) {
							this.stopTask(taskId, log.runId);
						}
					}
				}));
			}
			historyEl.append(el);
			el.onclick = () => {
				this.updateGraph(log);
				this.selectRun(log);
			};
		}
	}

	async updateTaskList() {
		let listEl = document.getElementById('task-list');
		listEl.innerText = '';

		let res = await fetch(apiUrl + 'tasks/');
		let tasks = await res.json();

		for (let t of tasks) {
			let el = mkEl('li', mkEl('a', t.taskId, { title: t.taskId, href: '#task:' + t.taskId }), { className: 'task' });
			el.dataset.taskId = t.taskId;
			listEl.append(el);
		}
	}

	setTaskFilter(filter) {
		let listEl = document.getElementById('task-list');
		for (let el of listEl.children) {
			if (el.dataset.taskId) {
				el.style.display = el.dataset.taskId.includes(filter) ? 'block' : 'none';
			}
		}
	}
}


window.addEventListener('DOMContentLoaded', (function (e) {
	let taskView = new TaskView();
	taskView.updateTaskList();

	function checkUrlFragment() {
		if (!location.hash) {
			return false;
		}
		let m = location.hash.match(/task:(.*)/)
		if (m) {
			taskView.updateTask(m[1]);
			return true;
		}
		return false;
	}
	checkUrlFragment();

	window.addEventListener('hashchange', (e) => {
		e.preventDefault();
		checkUrlFragment();
	}, false);

	function initPopup(buttonEl, popup, className) {
		buttonEl.addEventListener("click", (ev) => {
			ev.preventDefault();
			popup.classList.toggle(className);
			if (popup.classList.contains(className)) {
				setTimeout(function () {
					window.addEventListener('click', function dismiss(ev) {
						window.removeEventListener('click', dismiss, false);
						popup.classList.remove(className);
					}, false);
				}, 1);
			}
		});
	};
	initPopup(document.querySelector('#menu-button'), document.querySelector('#menu-pane'), "override_menu_visible");
	initPopup(document.querySelector('#option-menu-button'), document.querySelector("#option-menu"), "active");
}));
