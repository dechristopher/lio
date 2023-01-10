import Link from "next/link";
import React from "react";

export const Header = () => {
	return (
		<Link href="/">
			<div className="flex cursor-pointer leading-7">
				<div className="mr-2 font-bold text-xl italic">
					li
					<span className="octad-green">octad</span>
					.org
				</div>
				*alpha
			</div>
		</Link>
	);
};
