import React from "react";
import Image from "next/image";
import AboutContainer from "@/components/AboutContainer/AboutContainer";

export default function Rules() {
	return (
		<AboutContainer>
			<div>
				<div className="about-header">Rules</div>

				<div className="prose">
					All standard chess rules apply: en passant is allowed, pawns
					promote to any piece, and stalemates are a draw.
				</div>
				<div className="prose">
					The only catch, however, is that castling is possible
					between the king and any of its pieces on the starting rank
					before movement. The king will simply switch spaces with the
					castling piece in all cases except the far pawn, in which
					case the king will travel one space to the right, and the
					pawn will lie where the king was before.
				</div>
				<div className="prose">
					An example of white castling with their far pawn can be
					expressed as [ 1. c2 b3 2. O-O-O ... ] with the resulting
					structure leaving the knight on a1, a pawn on b1, the king
					on c1, and the other pawn on c2. Here is what that would
					look like on the board:
				</div>

				<table className="about-table">
					<thead>
						<tr>
							<th>1. c2</th>
							<th>1. c2 b3</th>
							<th>2. O-O-O</th>
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
									src="/images/far-castle.svg"
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
