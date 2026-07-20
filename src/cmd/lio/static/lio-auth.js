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
