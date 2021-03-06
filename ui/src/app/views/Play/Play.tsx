import React, {FC} from "react";

export const PlayView: FC = () => {
	return (
		<div className="mt-16 w-screen" style={{height: "calc(100vh - 4rem)"}}>
			<div className="md:flex md:items-center md:justify-between pt-8 pb-36 bg-green-600">
				<div className="px-6 flex-1 min-w-0">
					<h2 className="text-2xl font-bold leading-7 text-white sm:text-3xl sm:truncate">
						Play
					</h2>
				</div>
			</div>
			<div className="flex-1 relative z-0 flex overflow-hidden h-full w-full -mt-28">
				<main className="flex-1 relative z-0 overflow-y-auto focus:outline-none" tabIndex={0}>
					{/* <!-- Start main area--> */}
					<div className="absolute inset-0 py-6 px-2 sm:px-4 lg:px-4">
							<div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-2 gap-4 sm:gap-6 lg:gap-8">
								<div className="border-2 border-black border-dashed rounded-lg h-48" />
								<div className="border-2 border-black border-dashed rounded-lg h-48" />
								<div className="border-2 border-black border-dashed rounded-lg h-48" />
								<div className="border-2 border-black border-dashed rounded-lg h-48" />
						</div>
					</div>
					{/* <!-- End main area --> */}
				</main>
				<aside className="hidden relative lg:flex lg:flex-col flex-shrink-0 w-96">
					{/* <!-- Start secondary column (hidden on smaller screens) --> */}
					<div className="absolute inset-0 py-6 px-2 sm:px-4 lg:px-4">
						<div className="h-full">
							<div className="border-2 border-black border-dashed rounded-lg h-96" />
						</div>
					</div>
					{/* <!-- End secondary column --> */}
				</aside>
			</div>
		</div>
	)
}