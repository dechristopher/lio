// lio-auth.js — account/auth wiring for the header controls on every page:
// the login/register modal tabs + fetch submission (logged out) and the
// profile popover's log out action (logged in). Server endpoints live under
// /api/auth (arch/ACCOUNTS_AUTH_RATINGS.md). Loaded deferred, so the DOM is
// parsed by the time this runs.
(() => {
	'use strict';

	/**
	 * POST JSON to an /api/auth endpoint.
	 * @param {string} path
	 * @param {object|null} body
	 * @returns {Promise<{status: number, data: object}>}
	 */
	const post = async (path, body) => {
		const res = await fetch(path, {
			method: 'POST',
			headers: body ? { 'Content-Type': 'application/json' } : {},
			body: body ? JSON.stringify(body) : undefined,
		});
		let data = {};
		try { data = await res.json(); } catch (e) { /* 204 etc. */ }
		return { status: res.status, data };
	};

	/**
	 * Show an error line inside a form (or hide it when msg is falsy).
	 * @param {HTMLFormElement} form
	 * @param {string} [msg]
	 */
	const showError = (form, msg) => {
		const el = form.querySelector('[data-auth-error]');
		if (!el) { return; }
		el.textContent = msg || '';
		el.classList.toggle('hidden', !msg);
	};

	// --- logged-in: profile popover actions --------------------------------
	const logoutBtn = document.getElementById('logoutButton');
	if (logoutBtn) {
		logoutBtn.addEventListener('click', async () => {
			logoutBtn.disabled = true;
			try { await post('/api/auth/logout', null); } catch (e) { /* still reload */ }
			window.location.reload();
		});
	}

	// log out everywhere: revoke all sessions (incl. this one) then reload
	const logoutAllBtn = document.getElementById('logoutAllButton');
	if (logoutAllBtn) {
		logoutAllBtn.addEventListener('click', async () => {
			logoutAllBtn.disabled = true;
			try { await post('/api/auth/logout-all', null); } catch (e) { /* still reload */ }
			window.location.reload();
		});
	}

	// change password: confirm client-side, POST, then show success (the server
	// signs out the account's other sessions; this one stays)
	const pwForm = document.getElementById('passwordForm');
	if (pwForm) {
		const okEl = pwForm.querySelector('[data-auth-ok]');
		pwForm.addEventListener('submit', async (e) => {
			e.preventDefault();
			showError(pwForm);
			if (okEl) { okEl.classList.add('hidden'); }
			if (pwForm.new.value !== pwForm.confirm.value) {
				showError(pwForm, 'New passwords do not match.');
				return;
			}
			try {
				const { status, data } = await post('/api/auth/password', {
					current: pwForm.current.value,
					new: pwForm.new.value,
				});
				if (status === 204) {
					pwForm.reset();
					if (okEl) { okEl.classList.remove('hidden'); }
					return;
				}
				showError(pwForm, data.error || 'Could not change password.');
			} catch (err) {
				showError(pwForm, 'Network error — try again.');
			}
		});
	}

	// active sessions: lazy-load the fragment the first time the section opens,
	// and reload it after a revoke (event-delegated on the list body)
	const sessionsDetails = document.getElementById('sessionsDetails');
	const sessionsBody = document.getElementById('sessionsBody');
	if (sessionsDetails && sessionsBody) {
		const loadSessions = async () => {
			try {
				const res = await fetch('/api/auth/sessions');
				sessionsBody.innerHTML = res.ok
					? await res.text()
					: '<p class="auth-hint">Could not load sessions.</p>';
				sessionsBody.dataset.loaded = 'true';
			} catch (err) {
				sessionsBody.innerHTML = '<p class="auth-hint">Could not load sessions.</p>';
			}
		};
		sessionsDetails.addEventListener('toggle', () => {
			if (sessionsDetails.open && sessionsBody.dataset.loaded !== 'true') {
				loadSessions();
			}
		});
		sessionsBody.addEventListener('click', async (e) => {
			const btn = e.target.closest('[data-session-id]');
			if (!btn) { return; }
			btn.disabled = true;
			try {
				await post('/api/auth/sessions/revoke', { id: Number(btn.dataset.sessionId) });
			} catch (err) { /* reload reflects the result either way */ }
			await loadSessions();
		});
	}

	// --- logged-out: auth modal --------------------------------------------
	const modal = document.getElementById('modalAccount');
	if (!modal) { return; }

	const tabs = modal.querySelectorAll('[data-auth-tab]');
	const forms = {
		login: modal.querySelector('[data-auth-form="login"]'),
		register: modal.querySelector('[data-auth-form="register"]'),
	};

	// tab switching: swap the active pill and the visible form. Forms toggle
	// between hidden and flex (they are flex columns when shown).
	tabs.forEach((tab) => {
		tab.addEventListener('click', () => {
			const which = tab.dataset.authTab;
			tabs.forEach((t) => t.classList.toggle('is-active', t === tab));
			Object.keys(forms).forEach((k) => {
				const form = forms[k];
				if (!form) { return; }
				form.classList.toggle('hidden', k !== which);
				form.classList.toggle('flex', k === which);
			});
			const first = forms[which] && forms[which].querySelector('input');
			if (first) { first.focus(); }
		});
	});

	// login submit
	if (forms.login) {
		forms.login.addEventListener('submit', async (e) => {
			e.preventDefault();
			const form = forms.login;
			showError(form);
			const body = {
				username: form.username.value.trim(),
				password: form.password.value,
			};
			try {
				const { status, data } = await post('/api/auth/login', body);
				if (status === 200) {
					window.location.reload();
					return;
				}
				showError(form, data.error || 'Login failed — try again.');
			} catch (err) {
				showError(form, 'Network error — try again.');
			}
		});
	}

	// register submit + live username availability probe (debounced)
	if (forms.register) {
		const form = forms.register;
		const availEl = form.querySelector('[data-auth-avail]');
		let availTimer = null;

		form.username.addEventListener('input', () => {
			if (!availEl) { return; }
			availEl.textContent = '';
			availEl.classList.remove('text-win', 'text-loss');
			clearTimeout(availTimer);
			const u = form.username.value.trim();
			if (u.length < 3) { return; }
			availTimer = setTimeout(async () => {
				try {
					const res = await fetch(
						'/api/auth/username-available?u=' + encodeURIComponent(u));
					const data = await res.json();
					if (form.username.value.trim() !== u) { return; } // stale
					if (data.available) {
						availEl.textContent = u + ' is available';
						availEl.classList.add('text-win');
					} else {
						availEl.textContent = data.reason || 'unavailable';
						availEl.classList.add('text-loss');
					}
				} catch (err) { /* probe is best-effort */ }
			}, 400);
		});

		form.addEventListener('submit', async (e) => {
			e.preventDefault();
			showError(form);
			const body = {
				username: form.username.value.trim(),
				password: form.password.value,
				email: form.email.value.trim(),
			};
			try {
				const { status, data } = await post('/api/auth/register', body);
				if (status === 200) {
					window.location.reload();
					return;
				}
				showError(form, data.error || 'Registration failed — try again.');
			} catch (err) {
				showError(form, 'Network error — try again.');
			}
		});
	}
})();
