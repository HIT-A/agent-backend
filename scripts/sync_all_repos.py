#!/usr/bin/env python3
"""
Sync all 18 HIT-related repositories to HITA_RagData
"""

import json
import subprocess
import time
import requests

# 所有仓库列表
REPOS = [
    "hitlug/hit-network-resources",
    "guoJohnny/-837-",
    "hithesis/hithesis",
    "rccoder/HIT-Computer-Network",
    "HITSZ-OpenCS/HITSZ-OpenCS",
    "PKUanonym/REKCARC-TSC-UHT",
    "QSCTech/zju-icicles",
    "HITLittleZheng/HITCS",
    "LiYing0/CS_Gra-HITsz",
    "hoverwinter/HIT-OSLab",
    "sherlockqwq/HITWH_learningResource_share",
    "gzn00417/HIT-CS-Labs",
    "DWaveletT/HIT-COA-Lab",
    "hitszosa/universal-hit-thesis",
    "gcentqs/hit-854",
    "szxSpark/hit-master-course-note",
    "Mor-Li/HITSZ-OpenDS",
    "HIT-A/HITA_RagData",  # Include the original repo too
]

API_URL = "http://localhost:8080"


def sync_repos():
    """Sync all repositories"""

    sources = []
    for repo in REPOS:
        sources.append(
            {
                "type": "github",
                "repo": repo,
                "file_types": [".md", ".txt", ".pdf"],
                "max_files": 100,
            }
        )

    payload = {
        "input": {
            "sources": sources,
            "local_path": "/tmp/HITA_RagData_All_18_Repos",
            "store_in_cos": False,
            "workers": 8,
        }
    }

    print(f"=== Syncing {len(sources)} repositories ===")
    print("This may take several minutes...")
    print()

    # Start the job
    resp = requests.post(
        f"{API_URL}/v1/skills/rag.sync_to_repo:invoke",
        headers={"Content-Type": "application/json"},
        json=payload,
    )
    data = resp.json()
    job_id = data.get("job_id")

    if not job_id:
        print("❌ Failed to start job")
        print(resp.text)
        return

    print(f"Job ID: {job_id}")
    print()

    # Poll status
	status = "unknown"
	output = {}
	error = ""
	
	for i in range(60):
        time.sleep(10)

        resp = requests.get(f"{API_URL}/v1/jobs/{job_id}")
        data = resp.json()
        job = data.get("job", {})
        status = job.get("status")
        output = job.get("output_json", {})
        error = job.get("error", "")

        total_files = output.get("total_files", 0)
        total_chunks = output.get("total_chunks", 0)

        elapsed = i * 10
        print(
            f"[{elapsed // 60:02d}:{elapsed % 60:02d}] Status: {status:10s} Files: {total_files:5d} Chunks: {total_chunks:5d}"
        )

        if status in ["succeeded", "failed"]:
            break

    print()
    print("=== Final Result ===")
    print(f"Status: {status}")
    print(f"Total Files: {output.get('total_files', 0)}")
    print(f"Total Chunks: {output.get('total_chunks', 0)}")
    print(f"Commit Hash: {output.get('commit_hash', 'N/A')}")
    if error:
        print(f"Error: {error}")

    # Show repo stats
    if output.get("sources_count"):
        print(f"Sources Processed: {output.get('sources_count')}")

    return status == "succeeded"


if __name__ == "__main__":
    success = sync_repos()
    if success:
        print("\n✅ All repositories synced successfully!")
        print("Check https://github.com/HIT-A/HITA_RagData for the results")
    else:
        print("\n❌ Sync failed")
