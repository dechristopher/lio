import React, {FC, useRef, useState} from "react";
import {Transition} from "@headlessui/react";
import {Link} from "react-router-dom";
import classNames from "classnames";

import LogoPrimary from "@assets/images/logo.svg";
import LogoAlt from "@assets/images/logo-alt.svg";
import {useOutsideClick} from "@/src/utils/hooks/useOutsideClick";
import {MobileNavbar} from "@app/components/Navbar/MobileNavbar";

export interface NavbarProps {
	pathname?: string;
}

export const Navbar: FC<NavbarProps> = (props) => {
	const [ menuOpen, setMenuOpen ] = useState<boolean>(false);
	const menuRef = useRef(null);
	const profileImgRef = useRef(null);
	const mobileMenuBtnRef = useRef(null);
	const mobileMenuRef = useRef(null);

	useOutsideClick([menuRef, profileImgRef, mobileMenuBtnRef, mobileMenuRef], () => {
		if (menuOpen) setMenuOpen(false)
	})

	return (
		<nav className="bg-white relative">
			<div className="fixed top-0 w-screen bg-white z-20 shadow">
				<div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
					<div className="flex justify-between h-16">
						<div className="flex">
							<div className="-ml-2 mr-2 flex items-center md:hidden">
								{/* Mobile menu button */}
								<button
									ref={mobileMenuBtnRef}
									type="button"
									aria-expanded="false"
									aria-controls="mobile-menu"
									onClick={() => setMenuOpen(open => !open)}
									className="inline-flex items-center justify-center p-2 rounded-md text-gray-400 hover:text-gray-500 hover:bg-gray-100 focus:outline-none focus:ring-2 focus:ring-inset focus:ring-green-500"
								>
									<span className="sr-only">Open main menu</span>
									{/* Icon when menu is closed. */}
									{/*
									Heroicon name: outline/menu

									Menu open: "hidden", Menu closed: "block"
								*/}
									<svg
										className={classNames("h-6 w-6", {
											"hidden": menuOpen,
											"block": !menuOpen,
										})}
										xmlns="http://www.w3.org/2000/svg"
										fill="none"
										viewBox="0 0 24 24"
										stroke="currentColor"
										aria-hidden="true"
									>
										<path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M4 6h16M4 12h16M4 18h16" />
									</svg>
									{/* Icon when menu is open. */}
									{/*
									Heroicon name: outline/x

									Menu open: "block", Menu closed: "hidden"
								*/}
									<svg className={classNames("h-6 w-6", {
										"hidden": !menuOpen,
										"block": menuOpen,
									})}  xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor" aria-hidden="true">
										<path strokeLinecap="round" strokeLinejoin="round" strokeWidth="2" d="M6 18L18 6M6 6l12 12" />
									</svg>
								</button>
							</div>
							<div className="flex-shrink-0 flex items-center">
								<LogoAlt className="block lg:hidden h-8 w-8" />
								<LogoPrimary className="hidden lg:block h-12 w-48 mt-1" />
							</div>
							<div className="hidden md:ml-6 md:flex md:space-x-8">
								{/* Current: "border-green-500 text-gray-900", Default: "border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700" */}
								<Link
									to="/play"
									className={classNames("inline-flex items-center px-1 pt-1 border-b-2 text-sm font-medium", {
										"border-green-500 text-gray-900": props.pathname === "/play",
										"border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700": props.pathname !== "/play"
									})}
								>
									Play
								</Link>
								<Link
									to="/learn"
									className={classNames("inline-flex items-center px-1 pt-1 border-b-2 text-sm font-medium", {
										"border-green-500 text-gray-900": props.pathname === "/learn",
										"border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700": props.pathname !== "/learn"
									})}
								>
									Learn
								</Link>
								<Link
									to="/watch"
									className={classNames("inline-flex items-center px-1 pt-1 border-b-2 text-sm font-medium", {
										"border-green-500 text-gray-900": props.pathname === "/watch",
										"border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700": props.pathname !== "/watch"
									})}
								>
									Watch
								</Link>
								<Link
									to="/players"
									className={classNames("inline-flex items-center px-1 pt-1 border-b-2 text-sm font-medium", {
										"border-green-500 text-gray-900": props.pathname === "/players",
										"border-transparent text-gray-500 hover:border-gray-300 hover:text-gray-700": props.pathname !== "/players"
									})}
								>
									Players
								</Link>
							</div>
						</div>
						<div className="flex items-center">
							<div className="flex-shrink-0">
								<button type="button" className="relative inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-white bg-green-600 shadow-sm hover:bg-green-700 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500">
									{/* Heroicon name: solid/plus */}
									<svg className="-ml-1 mr-2 h-5 w-5" viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg">
										<path d="M17 17L12.3333 12.3333M13.8889 8.44444C13.8889 11.4513 11.4513 13.8889 8.44444 13.8889C5.43756 13.8889 3 11.4513 3 8.44444C3 5.43756 5.43756 3 8.44444 3C11.4513 3 13.8889 5.43756 13.8889 8.44444Z" stroke="white" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/>
									</svg>
									<span>Find Game</span>
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
										<button
											id="user-menu"
											ref={profileImgRef}
											aria-haspopup="true"
											onClick={() => setMenuOpen(open => !open)}
											className="bg-white rounded-full flex text-sm focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-500"
										>
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
									<Transition
										show={menuOpen}
										enter="transition ease-out duration-200"
										enterFrom="transform opacity-0 scale-95"
										enterTo="transform opacity-100 scale-100"
										leave="transition ease-in duration-75"
										leaveFrom="transform opacity-100 scale-100"
										leaveTo="transform opacity-0 scale-95"
									>
										<div
											role="menu"
											ref={menuRef}
											aria-orientation="vertical"
											aria-labelledby="user-menu"
											className="origin-top-right absolute right-0 mt-2 w-48 rounded-md shadow-lg py-1 bg-white ring-1 ring-black ring-opacity-5"
										>
											<Link to="/u" className="block px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" role="menuitem">Profile</Link>
											<Link to="/account/settings" className="block px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" role="menuitem">Settings</Link>
											<Link to="/sign-out" className="block px-4 py-2 text-sm text-gray-700 hover:bg-gray-100" role="menuitem">Sign out</Link>
										</div>
									</Transition>
								</div>
							</div>
						</div>
					</div>
				</div>
			</div>

			{/* Mobile menu, show/hide based on menu state. */}
			<div className="md:hidden absolute w-screen z-10" style={{top: "4.25rem"}} id="mobile-menu">
				<MobileNavbar {...props} menuOpen={menuOpen} setMenuOpen={setMenuOpen} menuRef={mobileMenuRef} />
			</div>
		</nav>
	)
}