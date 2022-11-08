import React from "react";
import Image from "next/image";
import AboutContainer from "@/components/AboutContainer/AboutContainer";

export default function Board() {
	return (
		<AboutContainer>
			<div>
				<div className="font-bold mb-3 text-sm">Board Layout</div>

				<div className="prose mb-3">
					Each player begins with four pieces: a knight, their king,
					and two pawns placed in that order from left to right
					relative to them. An example of this can be seen in the
					board diagrams below:
				</div>

				<table className="about-table">
					<thead>
						<tr>
							<th>1. c2</th>
							<th>1. c2 b3</th>
							<th>2. cxb3!</th>
						</tr>
					</thead>
					<tbody>
						<tr>
							<td>
								<Image
									width="100%"
									height="100%"
									src="/images/octad2.svg"
									alt="octad board layout 2"
								/>
							</td>
							<td>
								<Image
									width="100%"
									height="100%"
									src="/images/octad3.svg"
									alt="octad board layout 3"
								/>
							</td>
							<td>
								<Image
									width="100%"
									height="100%"
									src="/images/octad4.svg"
									alt="octad board layout 4"
								/>
							</td>
						</tr>
					</tbody>
				</table>
			</div>
		</AboutContainer>
	);
}
