import React, {FC, MutableRefObject} from "react";
import {NavbarProps} from "@app/components/Navbar/Navbar";
import {Link} from "react-router-dom";
import classNames from "classnames";
import {Transition} from "@headlessui/react";

export interface MobileNavbarProps extends NavbarProps {
	menuOpen: boolean;
	menuRef: MutableRefObject<null>;
	setMenuOpen: React.Dispatch<React.SetStateAction<boolean>>;
}

export const MobileNavbar: FC<MobileNavbarProps> = (props) => {
	return (
		<Transition
			show={props.menuOpen}
			enter="transform transition ease-in-out duration-200"
			enterFrom="-translate-y-full"
			enterTo="translate-y-0"
			leave="transform transition ease-in-out duration-200"
			leaveFrom="translate-y-0"
			leaveTo="-translate-y-full"
		>
			<div className="bg-white" ref={props.menuRef}>
				<div className="pt-2 pb-3 space-y-1">
					{/* Current: "bg-green-50 border-green-500 text-green-700", Default: "border-transparent text-gray-500 hover:bg-gray-50 hover:border-gray-300 hover:text-gray-700" */}
					<Link
						to="/play"
						className={classNames("block pl-3 pr-4 py-2 border-l-4 text-base font-medium sm:pl-5 sm:pr-6", {
							"bg-green-50 border-green-500 text-green-700": props.pathname === "/play",
							"border-transparent text-gray-500 hover:bg-gray-50 hover:border-gray-300 hover:text-gray-700": props.pathname !== "/play"
						})}
					>
						Play
					</Link>
					<Link
						to="/learn"
						className={classNames("block pl-3 pr-4 py-2 border-l-4 text-base font-medium sm:pl-5 sm:pr-6", {
							"bg-green-50 border-green-500 text-green-700": props.pathname === "/learn",
							"border-transparent text-gray-500 hover:bg-gray-50 hover:border-gray-300 hover:text-gray-700": props.pathname !== "/learn"
						})}
					>
						Learn
					</Link>
					<Link
						to="/watch"
						className={classNames("block pl-3 pr-4 py-2 border-l-4 text-base font-medium sm:pl-5 sm:pr-6", {
							"bg-green-50 border-green-500 text-green-700": props.pathname === "/watch",
							"border-transparent text-gray-500 hover:bg-gray-50 hover:border-gray-300 hover:text-gray-700": props.pathname !== "/watch"
						})}
					>
						Watch
					</Link>
					<Link
						to="/players"
						className={classNames("block pl-3 pr-4 py-2 border-l-4 text-base font-medium sm:pl-5 sm:pr-6", {
							"bg-green-50 border-green-500 text-green-700": props.pathname === "/players",
							"border-transparent text-gray-500 hover:bg-gray-50 hover:border-gray-300 hover:text-gray-700": props.pathname !== "/players"
						})}
					>
						Players
					</Link>
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
		</Transition>
	)
}