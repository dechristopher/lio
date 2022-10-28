import Link from "next/link";
import React from "react";

export const Header = () => {
	return (
		<div className="cursor-pointer">
			<Link href="/">
				<div className="flex">
					<div className="mr-2 font-bold text-xl italic">
						li
						<span
							style={{
								color: "#0bab7d",
							}}
						>
							octad
						</span>
						.org
					</div>
					*alpha
				</div>
			</Link>
		</div>
	);
};
