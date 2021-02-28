import React, {FC} from "react";
// import {Link} from "react-router-dom";

import LogoPrimary from "@assets/images/logo.svg";

export interface NavbarProps {
	currentPath?: string;
}

export const Navbar: FC<NavbarProps> = () => {
	return (
		<nav className="bg-white shadow">
			<div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
				<div className="flex justify-between h-16">
					<div className="flex">
						<div className="-ml-2 mr-2 flex items-center md:hidden">
							{/* Mobile menu button */}
							<button type="button" className="inline-flex items-center justify-center p-2 rounded-md text-gray-400 hover:text-gray-500 hover:bg-gray-100 focus:outline-none focus:ring-2 focus:ring-inset focus:ring-green-500" aria-controls="mobile-menu" aria-expanded="false">
								<span className="sr-only">Open main menu</span>
								{/* Icon when menu is closed. */}
								{/*
									Heroicon name: outline/menu

									Menu open: "hidden", Menu closed: "block"
								*/}
								<svg className="block h-6 w-6" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true">
									<path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M4 6h16M4 12h16M4 18h16" />
								</svg>
								{/* Icon when menu is open. */}
								{/*
									Heroicon name: outline/x

									Menu open: "block", Menu closed: "hidden"
								*/}
								<svg className="hidden h-6 w-6" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true">
									<path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M6 18L18 6M6 6l12 12" />
								</svg>
							</button>
						</div>
						<div className="flex-shrink-0 flex items-center">
							<LogoPrimary className="block lg:hidden h-8 w-auto" />
							<LogoPrimary className="hidden lg:block h-8 w-auto" />
						</div>
						<div className="hidden md:ml-6 md:flex md:space-x-8">
							{/* Current: "border-green-500 text-gray-900", Default: "border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700" */}
							{/*<Link to="/play" className="border-green-500 text-gray-900 inline-flex items-center px-1 pt-1 border-b-2 text-sm font-medium">*/}
							{/*	Play*/}
							{/*</Link>*/}
							{/*<Link to="/learn" className="border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700 inline-flex items-center px-1 pt-1 border-b-2 text-sm font-medium">*/}
							{/*	Learn*/}
							{/*</Link>*/}
							{/*<Link to="/watch" className="border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700 inline-flex items-center px-1 pt-1 border-b-2 text-sm font-medium">*/}
							{/*	Watch*/}
							{/*</Link>*/}
							{/*<Link to="/players" className="border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700 inline-flex items-center px-1 pt-1 border-b-2 text-sm font-medium">*/}
							{/*	Players*/}
							{/*</Link>*/}
						</div>
					</div>
					<div className="flex items-center">
						<div className="flex-shrink-0">
							<button type="button" className="relative inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-white bg-green-600 shadow-sm hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500">
								{/* Heroicon name: solid/plus */}
								<svg className="-ml-1 mr-2 h-5 w-5" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
									<path fillRule="evenodd" d="M10 5a1 1 0 011 1v3h3a1 1 0 110 2h-3v3a1 1 0 11-2 0v-3H6a1 1 0 110-2h3V6a1 1 0 011-1z" clipRule="evenodd" />
								</svg>
								<span>THANK FUCKING GOD</span>
							</button>
						</div>
						<div className="hidden md:ml-4 md:flex-shrink-0 md:flex md:items-center">
							<button className="bg-white p-1 rounded-full text-gray-400 hover:text-gray-500 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500">
								<span className="sr-only">View notifications</span>
								{/* Heroicon name: outline/bell */}
								<svg className="h-6 w-6" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true">
									<path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" />
								</svg>
							</button>

							{/* Profile dropdown */}
							<div className="ml-3 relative">
								<div>
									<button className="bg-white rounded-full flex text-sm focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500" id="user-menu" aria-haspopup="true">
										<span className="sr-only">Open user menu</span>
										<img className="h-8 w-8 rounded-full" src="https://images.unsplash.com/photo-1472099645785-5658abf4ff4e?ixlib=rb-1.2.1&ixqx=tlsAo3tXUW&ixid=eyJhcHBfaWQiOjEyMDd9&auto=format&fit=facearea&facepad=2&w=256&h=256&q=80" alt="" />
									</button>
								</div>
								{/*
									Profile dropdown panel, show/hide based on dropdown state.

									Entering: "transition ease-out duration-200"
										From: "transform opacity-0 scale-95"
										To: "transform opacity-100 scale-100"
									Leaving: "transition ease-in duration-75"
										From: "transform opacity-100 scale-100"
										To: "transform opacity-0 scale-95"
								*/}
								<div className="origin-top-right absolute right-0 mt-2 w-48 rounded-md shadow-lg py-1 bg-white ring-1 ring-black ring-opacity-5" role="menu" aria-orientation="vertical" aria-labelledby="user-menu">
									<a href="#" className="block px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" role="menuitem">Profile</a>
									<a href="#" className="block px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" role="menuitem">Settings</a>
									<a href="#" className="block px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" role="menuitem">Sign out</a>
								</div>
							</div>
						</div>
					</div>
				</div>
			</div>

			{/* Mobile menu, show/hide based on menu state. */}
			<div className="md:hidden" id="mobile-menu">
				<div className="pt-2 pb-3 space-y-1">
					{/* Current: "bg-green-50 border-green-500 text-green-700", Default: "border-transparent text-gray-500 hover:bg-gray-50 hover:border-gray-300 hover:text-gray-700" */}
					{/*<Link to="/players" className="bg-green-50 border-green-500 text-green-700 block pl-3 pr-4 py-2 border-l-4 text-base font-medium sm:pl-5 sm:pr-6">Play</Link>*/}
					{/*<Link to="/learn" className="border-transparent text-gray-500 hover:bg-gray-50 hover:border-gray-300 hover:text-gray-700 block pl-3 pr-4 py-2 border-l-4 text-base font-medium sm:pl-5 sm:pr-6">Learn</Link>*/}
					{/*<Link to="/watch" className="border-transparent text-gray-500 hover:bg-gray-50 hover:border-gray-300 hover:text-gray-700 block pl-3 pr-4 py-2 border-l-4 text-base font-medium sm:pl-5 sm:pr-6">Watch</Link>*/}
					{/*<Link to="/players" className="border-transparent text-gray-500 hover:bg-gray-50 hover:border-gray-300 hover:text-gray-700 block pl-3 pr-4 py-2 border-l-4 text-base font-medium sm:pl-5 sm:pr-6">Players</Link>*/}
				</div>
				<div className="pt-4 pb-3 border-t border-gray-200">
					<div className="flex items-center px-4 sm:px-6">
						<div className="flex-shrink-0">
							<img className="h-10 w-10 rounded-full" src="https://images.unsplash.com/photo-1472099645785-5658abf4ff4e?ixlib=rb-1.2.1&ixqx=tlsAo3tXUW&ixid=eyJhcHBfaWQiOjEyMDd9&auto=format&fit=facearea&facepad=2&w=256&h=256&q=80" alt="" />
						</div>
						<div className="ml-3">
							<div className="text-base font-medium text-gray-800">Tom Cook</div>
							<div className="text-sm font-medium text-gray-500">tom@example.com</div>
						</div>
						<button className="ml-auto flex-shrink-0 bg-white p-1 rounded-full text-gray-400 hover:text-gray-500 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500">
							<span className="sr-only">View notifications</span>
							{/* Heroicon name: outline/bell */}
							<svg className="h-6 w-6" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true">
								<path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" />
							</svg>
						</button>
					</div>
					<div className="mt-3 space-y-1">
						<a href="#" className="block px-4 py-2 text-base font-medium text-gray-500 hover:text-gray-800 hover:bg-gray-100 sm:px-6">Your Profile</a>
						<a href="#" className="block px-4 py-2 text-base font-medium text-gray-500 hover:text-gray-800 hover:bg-gray-100 sm:px-6">Settings</a>
						<a href="#" className="block px-4 py-2 text-base font-medium text-gray-500 hover:text-gray-800 hover:bg-gray-100 sm:px-6">Sign out</a>
					</div>
				</div>
			</div>
		</nav>
	)
}