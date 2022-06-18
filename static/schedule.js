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


class ScheduleView {
	constructor() {
	}


	async updateList() {
		let listEl = document.getElementById('schedule-list');
		listEl.innerHTML = '';
		let res = await fetch(apiUrl + 'schedules/');
		if (!res.ok) {
			this.error('failed to fetch schedules.');
			return;
		}
		this.error('');
		for (let sch of await res.json()) {
			listEl.append(mkEl('ul', [sch.schedule, sch.taskId, mkEl('button', 'x', { onclick: () => { this.setSchedule(sch.taskId, ''); } })]));
		}
	}

	async setSchedule(taskId, schedule) {
		let data = new FormData();
		data.append("taskId", taskId);
		data.append("schedule", schedule);
		let res = await fetch(apiUrl + 'schedules/', { method: "POST", body: data });
		if (!res.ok) {
			this.error('Failed to set schedule for ' + taskId);
			return;
		}
		this.updateList();
	}

	error(msg) {
		let el = document.getElementById('error');
		el.innerText = msg;
		el.style.display = msg ? 'block' : 'none';
	}
}


window.addEventListener('DOMContentLoaded', (function (e) {
	let scheduleView = new ScheduleView();

	scheduleView.updateList();

	document.getElementById('refresh-button').addEventListener('click', async (e) => {
		document.getElementById('schedule-add-form').style.display = 'none';
		scheduleView.updateList();
	});


	document.getElementById('schedule-add-button').addEventListener('click', async (e) => {
		e.preventDefault();
		document.getElementById('schedule-add-form').style.display = 'block';
		document.getElementById('schedule-add-form-schedule').value = "30 12 * * *";
		let res = await fetch(apiUrl + 'tasks/');
		if (!res.ok) {
			scheduleView.error('failed to fetch task list.');
			return;
		}
		let tasks = await res.json();
		let selectEl = document.getElementById('schedule-add-form-taskid');
		selectEl.innerHTML = '';
		scheduleView.error('');
		for (let t of tasks) {
			selectEl.append(mkEl('option', t.taskId, { value: t.taskId }));
		}
	});

	document.getElementById('schedule-add-form-add').addEventListener('click', (e) => {
		e.preventDefault();
		scheduleView.setSchedule(document.getElementById('schedule-add-form-taskid').value, document.getElementById('schedule-add-form-schedule').value);
		document.getElementById('schedule-add-form').style.display = 'none';
	});

	let initPopup = function (button, popup, className) {
		button.addEventListener("click", (ev) => {
			ev.preventDefault();
			ev.stopPropagation();
			popup.classList.toggle(className);
			if (popup.classList.contains(className)) {
				setTimeout(function () {
					window.addEventListener('click', function dismiss(ev) {
						window.removeEventListener('click', dismiss, false);
						if (popup.classList.contains(className)) {
							popup.classList.remove(className);
						}
					}, false);
				}, 1);
			}
		});
	};
	initPopup(document.querySelector('#menu-button'), document.querySelector('#menu-pane'), "override_menu_visible");
	initPopup(document.querySelector('#option-menu-button'), document.querySelector("#option-menu"), "active");
	initPopup(document.querySelector('#item-sort-button'), document.querySelector("#sort-order-list"), "active");
}));
