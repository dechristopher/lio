import React, {FC, useRef, useState} from "react";
import {useHistory} from "react-router-dom";
import {Transition} from "@headlessui/react";

import LogoPrimary from "@assets/images/logo.svg";
import {useOutsideClick} from "@/src/utils/hooks/useOutsideClick";
import {DesktopNavLink} from "@app/components/Navbar/desktop/DesktopNavLink";
import {MobileNavbarMenu} from "@app/components/Navbar/mobile/MobileNavbarMenu";
import {DesktopMenuOption} from "@app/components/Navbar/desktop/DesktopMenuOption";
import {MobileNavbarMenuButton} from "@app/components/Navbar/mobile/MobileNavbarMenuButton";

export interface NavbarProps {
	pathname?: string;
}

export const Navbar: FC<NavbarProps> = (props) => {
	const history = useHistory();
	const [ menuOpen, setMenuOpen ] = useState<boolean>(false);

	const menuRef = useRef(null);
	const profileImgRef = useRef(null);
	const mobileMenuRef = useRef(null);
	const mobileMenuBtnRef = useRef(null);

	useOutsideClick([menuRef, profileImgRef, mobileMenuBtnRef, mobileMenuRef], () => {
		if (menuOpen) setMenuOpen(false)
	})

	return (
		<nav className="bg-white relative">
			<div className="fixed top-0 w-screen bg-white z-20 shadow">
				<div className="max-w-7xl mx-auto px-4 sm:px-6 lg:px-8">
					<div className="flex justify-between h-16">
						<div className="flex">
							<MobileNavbarMenuButton
								menuOpen={menuOpen}
								setMenuOpen={setMenuOpen}
								menuButtonRef={mobileMenuBtnRef}
							/>
							<div className="flex-shrink-0 flex items-center cursor-pointer" onClick={() => history.push("/")}>
								{/*<LogoAlt className="block md:hidden h-8 w-8" />*/}
								<LogoPrimary className="h-12 w-48 mt-1" />
							</div>
							<div className="hidden md:ml-6 md:flex md:space-x-8">
								<DesktopNavLink to="/play" active={props.pathname === "/play"}>Play</DesktopNavLink>
								<DesktopNavLink to="/learn" active={props.pathname === "/learn"}>Learn</DesktopNavLink>
								<DesktopNavLink to="/watch" active={props.pathname === "/watch"}>Watch</DesktopNavLink>
								<DesktopNavLink to="/players" active={props.pathname === "/players"}>Players</DesktopNavLink>
							</div>
						</div>
						<div className="flex items-center">
							<div className="flex-shrink-0">
								<button type="button" className="relative inline-flex items-center px-4 py-2 border border-transparent text-sm font-medium rounded-md text-white bg-green-500 shadow-sm hover:bg-green-600 focus:outline-none focus:ring-2 focus:ring-offset-2 focus:ring-green-400">
									{/* Heroicon name: solid/plus */}
									<svg className="-ml-1 mr-2 h-5 w-5" viewBox="0 0 20 20" fill="none" xmlns="http://www.w3.org/2000/svg">
										<path d="M17 17L12.3333 12.3333M13.8889 8.44444C13.8889 11.4513 11.4513 13.8889 8.44444 13.8889C5.43756 13.8889 3 11.4513 3 8.44444C3 5.43756 5.43756 3 8.44444 3C11.4513 3 13.8889 5.43756 13.8889 8.44444Z" stroke="white" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round"/>
									</svg>
									<span>Find game</span>
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

									{/* Profile dropdown panel, show/hide based on dropdown state. */}
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
											className="origin-top-right absolute right-0 mt-6 w-48 rounded-md shadow-lg py-1 bg-white ring-1 ring-black ring-opacity-5"
										>
											<DesktopMenuOption
												type="profile"
												title={<div className="text-base font-medium text-gray-800">Tom Cook</div>}
												description={<div className="text-sm font-medium text-gray-500">tom@example.com</div>}
											/>
											<DesktopMenuOption type="link" to="/u">Profile</DesktopMenuOption>
											<DesktopMenuOption type="link" to="/account/settings">Settings</DesktopMenuOption>
											<DesktopMenuOption type="link" to="/sign-out">Sign out</DesktopMenuOption>
										</div>
									</Transition>
								</div>
							</div>
						</div>
					</div>
				</div>
			</div>

			{/* Mobile menu, show/hide based on menu state. */}
			<div className="md:hidden absolute w-screen z-10 top-0" id="mobile-menu">
				<MobileNavbarMenu {...props} menuOpen={menuOpen} setMenuOpen={setMenuOpen} menuRef={mobileMenuRef} />
			</div>
		</nav>
	)
}