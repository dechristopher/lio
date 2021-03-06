import React, {FC} from "react";

export const PlayView: FC = () => {
	return (
		<div className="mt-16 w-screen flex flex-col items-stretch overflow-x-hidden overflow-y-auto" style={{height: "calc(100vh - 4rem)"}}>
			<div className="md:flex md:items-center md:justify-between pt-8 pb-36 bg-green-600">
				<div className="px-6 flex-1 min-w-0">
					<h2 className="text-2xl font-bold leading-7 text-white sm:text-3xl sm:truncate">
						Play
					</h2>
				</div>
			</div>
			<div className="flex-1 relative z-0 flex -mt-28">
				<main className="flex-1 relative z-0 focus:outline-none space-y-8" tabIndex={0}>
					<div className="absolute inset-0 py-6 px-2 sm:px-4 lg:px-4">
					{/* <!-- Start main area--> */}

						<div className="border-2 border-black border-dashed rounded-lg h-16 flex md:hidden">
							Online Player Stats
						</div>

						<div className="border-2 border-black border-dashed rounded-lg h-96 mt-8 md:mt-0">
								Variant Selector
						</div>

						<div className="border-2 border-black border-dashed rounded-lg h-96 mt-8">
							Custom Games
						</div>

						<div className="border-2 border-black border-dashed rounded-lg h-96 flex md:hidden mt-8 md:mt-0">
							Top Players
						</div>

						<div className="border-2 border-black border-dashed rounded-lg h-16 flex md:hidden mt-8 md:mt-0">
							Footer
						</div>
					{/* <!-- End main area --> */}
					</div>
				</main>
				<aside className="hidden relative md:flex md:flex-col flex-shrink-0 w-4/12">
					{/* <!-- Start secondary column (hidden on smaller screens) --> */}
					<div className="absolute inset-0 py-6 px-2 sm:px-4 lg:px-4 space-y-8">
						<div className="border-2 border-black border-dashed rounded-lg h-96">
							Top Players
						</div>

						<div className="border-2 border-black border-dashed rounded-lg h-16">
							Online Player Stats
						</div>

						<div className="border-2 border-black border-dashed rounded-lg h-16">
							Footer
						</div>
					</div>
					{/* <!-- End secondary column --> */}
				</aside>
			</div>
		</div>
	)
}